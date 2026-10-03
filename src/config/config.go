package config

import (
	"net"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type NodeConfig struct {
	ID int    `yaml:"id"`
	IP string `yaml:"ip"`
}

type LBConfig struct {
	VirtualIP   string `yaml:"virtual_ip"`
	ForwardPort int    `yaml:"forward_port"`
	Interface   string `yaml:"interface"`
}

type Config struct {
	ListenPort int          `yaml:"listen_port"`
	Nodes      []NodeConfig `yaml:"nodes"`
	LB         LBConfig     `yaml:"lb"`
	Self       int          `yaml:"self"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg Config) IDFromIP(ip string) (int, error) {
	host, _, _ := net.SplitHostPort(ip)

	for _, n := range cfg.Nodes {
		if n.IP == host {
			return n.ID, nil
		}
	}

	return 0, os.ErrNotExist
}

func (cfg Config) ListenPortString() string {
	return strconv.Itoa(cfg.ListenPort)
}
