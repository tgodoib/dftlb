# DFTLB — Distributed Fault-Tolerant Load Balancer

DFTLB is a distributed, fault-tolerant TCP load balancer without a fixed master,
designed to run on low-cost repurposed hardware (originally, seized TV Boxes
running Armbian). Every node runs the same binary. The nodes elect a master
among themselves, and if the master fails, a new one is elected automatically.

Packet forwarding is not done by DFTLB itself: it is delegated to the Linux
kernel's [IPVS](https://en.wikipedia.org/wiki/IP_Virtual_Server) in
Direct Routing mode. DFTLB only runs the control plane.

## How it works

- **Leader election:** nodes exchange heartbeats every 1 s over TCP. If no
  heartbeat arrives from the master for 3 s, an election is started using the
  Bully algorithm, and the node with the highest ID among the live ones becomes
  master.
- **Master:** creates the IPVS virtual service for the virtual IP (VIP) with
  round-robin scheduling, assigns the VIP to its network interface, announces
  it with gratuitous ARP and registers every other live node as a real server
  (backend) in Direct Routing mode.
- **Workers:** assign the VIP to the loopback interface and suppress ARP
  replies for it (`arp_ignore=1`, `arp_announce=2`), so that only the master
  answers for the VIP on the network. They reply to clients directly.
- **Membership changes:** when a node joins or leaves, the master adds or
  removes it from IPVS. If the master leaves, the remaining nodes elect a new
  one, which takes over the VIP.

Because balancing happens at the transport layer, DFTLB is agnostic to the
application served by the workers (any TCP service on `forward_port`).

## Requirements

On every node:

- Linux with the IPVS modules (`ip_vs`, `ip_vs_rr`, `ip_vs_dr`)
- `arping` (used for the gratuitous ARP announcements)
- Root privileges (IPVS, addresses on interfaces and sysctls are changed at runtime)
- The service to be balanced listening on `forward_port` (e.g., Nginx on port 80)
- TCP connectivity between nodes on `listen_port`

To build:

- Go 1.26 or newer

## Configuration

DFTLB reads `network.yaml` from its working directory (`/etc/dftlb` when run
via systemd). All nodes share the same file, except for `self`:

```yaml
listen_port: 9000        # TCP port used for node-to-node communication

lb:
  virtual_ip: 10.0.0.100 # VIP that clients connect to
  forward_port: 80       # port of the balanced service
  interface: end0        # network interface where the master assigns the VIP

nodes:                   # all cluster members (ID must be unique)
  - id: 1
    ip: 10.0.0.101
  - id: 2
    ip: 10.0.0.102
  - id: 3
    ip: 10.0.0.103

self: 3                  # ID of this node
```

Nodes are identified by their IP address, so each entry in `nodes` must match
the address the node uses to reach its peers.

## Build

From the `src/` directory, cross-compiling for 32-bit ARM (as used by the TV Boxes):

```bash
GOOS=linux GOARCH=arm GOARM=7 go build -o ../_out/dftlb .
```

For other targets, adjust `GOOS`/`GOARCH` (e.g., `GOARCH=arm64` or `GOARCH=amd64`).

## Install and run

On each node:

```bash
# binary
sudo cp _out/dftlb /usr/local/bin/dftlb

# configuration (edit `self` on each node)
sudo mkdir -p /etc/dftlb
sudo cp network.yaml /etc/dftlb/network.yaml

# kernel modules
sudo modprobe ip_vs ip_vs_rr ip_vs_dr

# systemd service
sudo cp dftlb.service /etc/systemd/system/dftlb.service
sudo systemctl daemon-reload
sudo systemctl enable --now dftlb
```

Logs are available with `journalctl -u dftlb -f`. To inspect the IPVS table on
the current master, use `sudo ipvsadm -Ln`.

## Project structure

```
src/
├── main.go        # wiring: config, election, heartbeat and role changes
├── config/        # network.yaml loading and logger
├── codec/         # message types and envelope encoding
├── transport/     # TCP server and peer connections
├── handlers/      # ping, heartbeat and Bully election
└── local/         # node state, message bus and IPVS/netlink control
network.yaml       # cluster configuration example
dftlb.service      # systemd unit
```

## Limitations

- Peers are identified by source IP address; there is no authentication or
  handshake between nodes.
- The master forwards traffic but is not registered as a backend itself.
- The failure detection time is fixed in code (1 s heartbeat, 3 s timeout).

## Reference

This project was developed at the Institute of Computing, University of
Campinas (Unicamp). A detailed description and evaluation are available in the
technical report IC-26-02: *Balanceador de carga distribuído tolerante a falhas
usando tv boxes reaproveitadas* (in Portuguese).
