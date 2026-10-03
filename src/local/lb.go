package local

import (
	"DFT_LB/config"
	"log"
	"log/slog"
	"net"
	"os"
	"os/exec"

	"github.com/moby/ipvs"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
)

type ipvsadm struct {
	handle   *ipvs.Handle
	svc      *ipvs.Service
	backends map[string]*ipvs.Destination
}

type netlinkIP struct {
	vipAddr *netlink.Addr
	ethLink netlink.Link
	loLink  netlink.Link
}

type LoadBalancer struct {
	cfg     config.LBConfig
	ipvs    *ipvsadm
	netlink *netlinkIP
	queue   chan func()
}

func NewLoadBalancer(cfg config.LBConfig) *LoadBalancer {
	handle, err := ipvs.New("")
	if err != nil {
		log.Fatal(err)
	}

	vipAddr, err := netlink.ParseAddr(cfg.VirtualIP + "/32")
	if err != nil {
		log.Fatal(err)
	}

	ethLink, err := netlink.LinkByName(cfg.Interface)
	if err != nil {
		log.Fatal(err)
	}

	loLink, err := netlink.LinkByName("lo")
	if err != nil {
		log.Fatal(err)
	}

	self := LoadBalancer{
		cfg: cfg,
		ipvs: &ipvsadm{
			handle: handle,
			svc: &ipvs.Service{
				Address:       net.ParseIP(cfg.VirtualIP).To4(),
				Protocol:      unix.IPPROTO_TCP,
				Port:          uint16(cfg.ForwardPort),
				SchedName:     "rr",
				Timeout:       0, // TODO: check usage
				AddressFamily: nl.FAMILY_V4,
			},
			backends: make(map[string]*ipvs.Destination),
		},
		netlink: &netlinkIP{
			vipAddr: vipAddr,
			ethLink: ethLink,
			loLink:  loLink,
		},
		queue: make(chan func(), 64),
	}

	go func() {
		for fn := range self.queue {
			fn()
		}
	}()

	return &self
}

func (lb *LoadBalancer) Enqueue(fn func()) {
	lb.queue <- fn
}

func (lb *LoadBalancer) BasicConfig() {
	slog.Debug("Basic IPVS Config")
	cmd := exec.Command("sudo", "apt", "install", "ipvsadm", "-y")
	if err := cmd.Run(); err != nil {
		log.Println("Error installing ipvsadm: ", err)
	}
	cmd = exec.Command("sudo", "apt", "install", "arping", "-y")
	if err := cmd.Run(); err != nil {
		log.Println("Error installing arping: ", err)
	}

	err := os.WriteFile("/proc/sys/net/ipv4/conf/all/arp_ignore", []byte("1"), 0644)
	if err != nil {
		log.Println("Error configuring arp_ignore: ", err)
	}
	err = os.WriteFile("/proc/sys/net/ipv4/conf/all/arp_announce", []byte("2"), 0644)
	if err != nil {
		log.Println("Error configuring arp_announce: ", err)
	}
	err = os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644)
	if err != nil {
		log.Println("Error configuring ip_forward: ", err)
	}
	err = os.WriteFile("/proc/sys/net/ipv4/conf/all/rp_filter", []byte("0"), 0644)
	if err != nil {
		log.Println("Error configuring all rp_filter: ", err)
	}
	err = os.WriteFile("/proc/sys/net/ipv4/conf/default/rp_filter", []byte("0"), 0644)
	if err != nil {
		log.Println("Error configuring default rp_filter: ", err)
	}

	cmd = exec.Command("modprobe", "ip_vs")
	if err := cmd.Run(); err != nil {
		log.Println("Error enabling ip_vs_dr: ", err)
	}
	cmd = exec.Command("modprobe", "ip_vs_dr")
	if err := cmd.Run(); err != nil {
		log.Println("Error enabling ip_vs_dr: ", err)
	}
}

func (lb *LoadBalancer) AcquireMaster() {
	slog.Debug("Acquiring master")
	//ipvsadm -C || true
	err := lb.ipvs.handle.Flush()
	if err != nil {
		slog.Error("ipvs.handle.Flush()" + err.Error())
	}

	//ipvsadm -A -t "${VIP}:${PORT}" -s rr
	err = lb.ipvs.handle.NewService(lb.ipvs.svc)
	if err != nil {
		slog.Error("ipvs.handle.NewService" + err.Error())
	}

	//ip addr add "${VIP}/32" dev "$IFACE"
	netlink.AddrDel(lb.netlink.ethLink, lb.netlink.vipAddr)
	err = netlink.AddrAdd(lb.netlink.ethLink, lb.netlink.vipAddr)
	if err != nil {
		slog.Error("netlink.AddrAdd " + err.Error())
	}

	lb.ArpMaster()
}

func (lb *LoadBalancer) ArpMaster() {
	go func() {
		//arping -U -c 3 -I $IFACE -S $VIP $VIP
		cmd := exec.Command("arping", "-U", "-c", "3", "-I", lb.cfg.Interface, "-S", lb.cfg.VirtualIP, lb.cfg.VirtualIP)
		if err := cmd.Run(); err != nil {
			slog.Warn("Running -U arping command: " + err.Error())
		}

		//arping -A -c 3 -I $IFACE -S $VIP $VIP
		cmd = exec.Command("arping", "-A", "-c", "3", "-I", lb.cfg.Interface, "-S", lb.cfg.VirtualIP, lb.cfg.VirtualIP)
		if err := cmd.Run(); err != nil {
			slog.Warn("Running -A arping command: " + err.Error())
		}
	}()
}

func (lb *LoadBalancer) ReleaseMaster() {
	slog.Debug("Releasing master")
	// ipvsadm -D -t "${VIP}:${PORT}"
	err := lb.ipvs.handle.DelService(lb.ipvs.svc)
	if err != nil {
		slog.Warn("ipvs.handle.DelService " + err.Error())
	}

	err = lb.ipvs.handle.Flush()
	if err != nil {
		slog.Warn("ipvs.handle.Flush() " + err.Error())
	}

	// ip addr del "${VIP}/32" dev "$IFACE"
	err = netlink.AddrDel(lb.netlink.ethLink, lb.netlink.vipAddr)
	if err != nil {
		slog.Warn("netlink.AddrDel " + err.Error())
	}
}

func (lb *LoadBalancer) AcquireFollower() {
	slog.Debug("Acquiring follower")
	//sysctl -w net.ipv4.conf.all.arp_ignore=1
	err := os.WriteFile("/proc/sys/net/ipv4/conf/all/arp_ignore", []byte("1"), 0644)
	if err != nil {
		slog.Error("os.WriteFile ", "arp_ignore", err.Error())
	}

	//sysctl -w net.ipv4.conf.all.arp_announce=2
	err = os.WriteFile("/proc/sys/net/ipv4/conf/all/arp_announce", []byte("2"), 0644)
	if err != nil {
		slog.Error("os.WriteFile ", "arp_announce", err.Error())
	}

	//ip addr add "${VIP}/32" dev lo
	err = netlink.AddrAdd(lb.netlink.loLink, lb.netlink.vipAddr)
	if err != nil {
		slog.Error("netlink.AddrAdd" + err.Error())
	}

	//ip route add "${VIP}/32" dev lo
	err = netlink.RouteAdd(&netlink.Route{
		LinkIndex: lb.netlink.loLink.Attrs().Index,
		Dst:       lb.netlink.vipAddr.IPNet,
	})
	if err != nil {
		slog.Error("netlink.RouteAdd" + err.Error())
	}
}

func (lb *LoadBalancer) ReleaseFollower() {
	slog.Debug("Releasing follower")

	//ip addr del "${VIP}/32" dev lo
	err := netlink.AddrDel(lb.netlink.loLink, lb.netlink.vipAddr)
	if err != nil {
		slog.Warn("netlink.AddrDel " + err.Error())
	}

	//ip route del "${VIP}/32" dev lo
	err = netlink.RouteDel(&netlink.Route{
		LinkIndex: lb.netlink.loLink.Attrs().Index,
		Dst:       lb.netlink.vipAddr.IPNet,
	})
	if err != nil {
		slog.Warn("netlink.RouteDel " + err.Error())
	}

	//sysctl -w net.ipv4.conf.all.arp_ignore=0
	err = os.WriteFile("/proc/sys/net/ipv4/conf/all/arp_ignore", []byte("0"), 0644)
	if err != nil {
		slog.Warn("os.WriteFile", "arp_ignore", err.Error())
	}

	//sysctl -w net.ipv4.conf.all.arp_announce=0
	err = os.WriteFile("/proc/sys/net/ipv4/conf/all/arp_announce", []byte("0"), 0644)
	if err != nil {
		slog.Warn("os.WriteFile", "arp_announce", err.Error())
	}
}

func (lb *LoadBalancer) RemoveBackend(peer string) {
	slog.Debug("Removing backend", "peer", peer)
	err := lb.ipvs.handle.DelDestination(lb.ipvs.svc, lb.ipvs.backends[peer])
	if err != nil {
		slog.Error("Removing peer as backend: " + err.Error())
	} else {
		slog.Debug("Successfully removed peer as backend")
	}
}

func (lb *LoadBalancer) AddBackend(peer string) {
	slog.Debug("Adding backend ", "peer", peer)

	host, _, err := net.SplitHostPort(peer)
	if err != nil {
		host = peer // already just an IP
	}

	dest := ipvs.Destination{
		Address:         net.ParseIP(host).To4(),
		Port:            uint16(lb.cfg.ForwardPort),
		AddressFamily:   nl.FAMILY_V4,
		Weight:          1,
		ConnectionFlags: ipvs.ConnFwdDirectRoute,
	}

	lb.ipvs.backends[peer] = &dest

	err = lb.ipvs.handle.NewDestination(lb.ipvs.svc, &dest)
	if err != nil {
		slog.Error("Adding peer as backend: " + err.Error())
	} else {
		slog.Debug("Successfully added peer as backend")
	}
}
