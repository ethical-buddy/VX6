package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const defaultListenAddress = "[::]:4242"

type File struct {
	Node  NodeConfig           `json:"node"`
	Peers map[string]PeerEntry `json:"peers"`
}

type NodeConfig struct {
	Name       string `json:"name"`
	ListenAddr string `json:"listen_addr"`
	DataDir    string `json:"data_dir"`
}

type PeerEntry struct {
	Address string `json:"address"`
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
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return filepath.Join(base, "vx6", "config.json"), nil
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

func defaultFile() File {
	return File{
		Node: NodeConfig{
			ListenAddr: defaultListenAddress,
			DataDir:    "./data/inbox",
		},
		Peers: map[string]PeerEntry{},
	}
}

func normalize(cfg *File) {
	if cfg.Node.ListenAddr == "" {
		cfg.Node.ListenAddr = defaultListenAddress
	}
	if cfg.Node.DataDir == "" {
		cfg.Node.DataDir = "./data/inbox"
	}
	if cfg.Peers == nil {
		cfg.Peers = map[string]PeerEntry{}
	}
}
