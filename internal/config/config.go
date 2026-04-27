package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/internal/transfer"
	vxtransport "github.com/vx6/vx6/internal/transport"
)

const defaultListenAddress = "[::]:4242"

const (
	FileReceiveOff     = "off"
	FileReceiveTrusted = "trusted"
	FileReceiveOpen    = "open"

	RelayModeOn  = "on"
	RelayModeOff = "off"
)

type File struct {
	Node     NodeConfig              `json:"node"`
	Peers    map[string]PeerEntry    `json:"peers"`
	Services map[string]ServiceEntry `json:"services"`
}

type NodeConfig struct {
	Name                 string   `json:"name"`
	ListenAddr           string   `json:"listen_addr"`
	AdvertiseAddr        string   `json:"advertise_addr"`
	TransportMode        string   `json:"transport_mode,omitempty"`
	HideEndpoint         bool     `json:"hide_endpoint"`
	RelayMode            string   `json:"relay_mode,omitempty"`
	RelayResourcePercent int      `json:"relay_resource_percent,omitempty"`
	DataDir              string   `json:"data_dir"`
	DownloadDir          string   `json:"download_dir"`
	BootstrapAddrs       []string `json:"bootstrap_addrs"`
	FileReceiveMode      string   `json:"file_receive_mode,omitempty"`
	AllowedFileSenders   []string `json:"allowed_file_senders,omitempty"`
}

type PeerEntry struct {
	Address string `json:"address"`
}

type ServiceEntry struct {
	Target        string   `json:"target"`
	IsHidden      bool     `json:"is_hidden"`
	Alias         string   `json:"alias,omitempty"`
	HiddenProfile string   `json:"hidden_profile,omitempty"`
	IntroMode     string   `json:"intro_mode,omitempty"`
	IntroNodes    []string `json:"intro_nodes,omitempty"`
}

type Store struct {
	path string
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = defaultPath
	}

	return &Store{path: path}, nil
}

func DefaultPath() (string, error) {
	if p := os.Getenv("VX6_CONFIG_PATH"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".config", "vx6", "config.json"), nil
}

func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "vx6"), nil
}

func DefaultDownloadDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "Downloads"), nil
}

func RuntimePIDPath(configPath string) (string, error) {
	if configPath == "" {
		var err error
		configPath, err = DefaultPath()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(filepath.Dir(configPath), "node.pid"), nil
}

func RuntimeLockPath(configPath string) (string, error) {
	if configPath == "" {
		var err error
		configPath, err = DefaultPath()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(filepath.Dir(configPath), "node.lock"), nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() (File, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultFile(), nil
		}
		return File{}, fmt.Errorf("read config: %w", err)
	}

	var cfg File
	if err := json.Unmarshal(data, &cfg); err != nil {
		return File{}, fmt.Errorf("decode config: %w", err)
	}
	normalize(&cfg)
	return cfg, nil
}

func (s *Store) Save(cfg File) error {
	normalize(&cfg)

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func (s *Store) AddPeer(name, address string) error {
	cfg, err := s.Load()
	if err != nil {
		return err
	}
	if err := record.ValidateNodeName(name); err != nil {
		return err
	}
	if err := transfer.ValidateIPv6Address(address); err != nil {
		return err
	}

	cfg.Peers[name] = PeerEntry{Address: address}
	return s.Save(cfg)
}

func (s *Store) ResolvePeer(name string) (PeerEntry, error) {
	cfg, err := s.Load()
	if err != nil {
		return PeerEntry{}, err
	}

	peer, ok := cfg.Peers[name]
	if !ok {
		return PeerEntry{}, fmt.Errorf("peer %q not found in %s", name, s.path)
	}

	return peer, nil
}

func (s *Store) ListPeers() ([]string, map[string]PeerEntry, error) {
	cfg, err := s.Load()
	if err != nil {
		return nil, nil, err
	}

	names := make([]string, 0, len(cfg.Peers))
	for name := range cfg.Peers {
		names = append(names, name)
	}
	sort.Strings(names)

	return names, cfg.Peers, nil
}

func (s *Store) AddBootstrap(address string) error {
	cfg, err := s.Load()
	if err != nil {
		return err
	}
	if err := transfer.ValidateIPv6Address(address); err != nil {
		return err
	}

	for _, existing := range cfg.Node.BootstrapAddrs {
		if existing == address {
			return s.Save(cfg)
		}
	}

	cfg.Node.BootstrapAddrs = append(cfg.Node.BootstrapAddrs, address)
	sort.Strings(cfg.Node.BootstrapAddrs)
	return s.Save(cfg)
}

func (s *Store) ListBootstraps() ([]string, error) {
	cfg, err := s.Load()
	if err != nil {
		return nil, err
	}

	out := append([]string(nil), cfg.Node.BootstrapAddrs...)
	sort.Strings(out)
	return out, nil
}

func (s *Store) AddService(name, target string, isHidden bool) error {
	cfg, err := s.Load()
	if err != nil {
		return err
	}

	cfg.Services[name] = ServiceEntry{Target: target, IsHidden: isHidden}
	return s.Save(cfg)
}

func (s *Store) SetService(name string, entry ServiceEntry) error {
	cfg, err := s.Load()
	if err != nil {
		return err
	}

	cfg.Services[name] = entry
	return s.Save(cfg)
}

func (s *Store) ResolveService(name string) (ServiceEntry, error) {
	cfg, err := s.Load()
	if err != nil {
		return ServiceEntry{}, err
	}

	service, ok := cfg.Services[name]
	if !ok {
		return ServiceEntry{}, fmt.Errorf("service %q not found in %s", name, s.path)
	}

	return service, nil
}

func (s *Store) ListServices() ([]string, map[string]ServiceEntry, error) {
	cfg, err := s.Load()
	if err != nil {
		return nil, nil, err
	}

	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	return names, cfg.Services, nil
}

func defaultFile() File {
	return File{
		Node: NodeConfig{
			ListenAddr:           defaultListenAddress,
			TransportMode:        vxtransport.ModeAuto,
			RelayMode:            RelayModeOn,
			RelayResourcePercent: 33,
			DataDir:              defaultDataDirValue(),
			DownloadDir:          defaultDownloadDirValue(),
		},
		Peers:    map[string]PeerEntry{},
		Services: map[string]ServiceEntry{},
	}
}

func normalize(cfg *File) {
	if cfg.Node.ListenAddr == "" {
		cfg.Node.ListenAddr = defaultListenAddress
	}
	if normalized := vxtransport.NormalizeMode(cfg.Node.TransportMode); normalized != "" {
		cfg.Node.TransportMode = normalized
	} else {
		cfg.Node.TransportMode = vxtransport.ModeAuto
	}
	if normalized := NormalizeRelayMode(cfg.Node.RelayMode); normalized != "" {
		cfg.Node.RelayMode = normalized
	} else {
		cfg.Node.RelayMode = RelayModeOn
	}
	cfg.Node.RelayResourcePercent = NormalizeRelayResourcePercent(cfg.Node.RelayResourcePercent)
	if cfg.Node.DataDir == "" || cfg.Node.DataDir == "./data/inbox" {
		cfg.Node.DataDir = defaultDataDirValue()
	}
	if cfg.Node.DownloadDir == "" {
		cfg.Node.DownloadDir = defaultDownloadDirValue()
	}
	if cfg.Node.BootstrapAddrs == nil {
		cfg.Node.BootstrapAddrs = []string{}
	}
	cfg.Node.FileReceiveMode = NormalizeFileReceiveMode(cfg.Node.FileReceiveMode)
	cfg.Node.AllowedFileSenders = uniqueSortedStrings(cfg.Node.AllowedFileSenders)
	if cfg.Node.FileReceiveMode != FileReceiveTrusted {
		cfg.Node.AllowedFileSenders = nil
	}
	if cfg.Peers == nil {
		cfg.Peers = map[string]PeerEntry{}
	}
	if cfg.Services == nil {
		cfg.Services = map[string]ServiceEntry{}
	}
	for name, svc := range cfg.Services {
		if svc.IntroNodes == nil {
			svc.IntroNodes = []string{}
		}
		if svc.IsHidden {
			if svc.HiddenProfile == "" {
				svc.HiddenProfile = "fast"
			}
			if svc.IntroMode == "" {
				if len(svc.IntroNodes) > 0 {
					svc.IntroMode = "manual"
				} else {
					svc.IntroMode = "random"
				}
			}
		}
		cfg.Services[name] = svc
	}
}

func NormalizeFileReceiveMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", FileReceiveOff:
		return FileReceiveOff
	case FileReceiveTrusted:
		return FileReceiveTrusted
	case FileReceiveOpen:
		return FileReceiveOpen
	default:
		return FileReceiveOff
	}
}

func NormalizeRelayMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", RelayModeOn:
		return RelayModeOn
	case RelayModeOff:
		return RelayModeOff
	default:
		return ""
	}
}

func NormalizeRelayResourcePercent(percent int) int {
	switch {
	case percent <= 0:
		return 33
	case percent < 5:
		return 5
	case percent > 90:
		return 90
	default:
		return percent
	}
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func defaultDataDirValue() string {
	path, err := DefaultDataDir()
	if err != nil {
		return filepath.Join(".", "vx6-data")
	}
	return path
}

func defaultDownloadDirValue() string {
	path, err := DefaultDownloadDir()
	if err != nil {
		return filepath.Join(".", "Downloads")
	}
	return path
}
