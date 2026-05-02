package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

type browserEntry struct {
	Target    string
	Title     string
	Args      []string
	Output    string
	Success   bool
	StartedAt time.Time
}

type browserView struct {
	ConfigPath    string
	CurrentTarget string
	CurrentTitle  string
	History       []browserEntry
	Bookmarks     []string
}

func (s *server) navigateBrowser(configPath, target string) error {
	entry, err := buildBrowserEntry(s.vx6Bin, configPath, target)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browserIndex >= 0 && s.browserIndex < len(s.browserHistory)-1 {
		s.browserHistory = append([]browserEntry(nil), s.browserHistory[:s.browserIndex+1]...)
	}
	s.browserHistory = append(s.browserHistory, entry)
	s.browserIndex = len(s.browserHistory) - 1
	s.browserCurrent = entry.Target
	if configPath = strings.TrimSpace(configPath); configPath != "" {
		s.browserConfigPath = configPath
	}
	s.applyBrowserEntryLocked(entry)
	return nil
}

func (s *server) browserBack() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browserIndex <= 0 || len(s.browserHistory) == 0 {
		return false
	}
	s.browserIndex--
	s.applyBrowserEntryLocked(s.browserHistory[s.browserIndex])
	return true
}

func (s *server) browserForward() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browserIndex < 0 || s.browserIndex >= len(s.browserHistory)-1 {
		return false
	}
	s.browserIndex++
	s.applyBrowserEntryLocked(s.browserHistory[s.browserIndex])
	return true
}

func (s *server) bookmarkBrowserTarget() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browserCurrent == "" {
		return false
	}
	return s.addBookmarkLocked(s.browserCurrent)
}

func (s *server) browserSnapshot() browserView {
	s.mu.Lock()
	defer s.mu.Unlock()

	view := browserView{
		ConfigPath:    s.browserConfigPath,
		CurrentTarget: s.browserCurrent,
		History:       append([]browserEntry(nil), s.browserHistory...),
		Bookmarks:     append([]string(nil), s.browserBookmarks...),
	}
	if s.browserIndex >= 0 && s.browserIndex < len(s.browserHistory) {
		view.CurrentTitle = s.browserHistory[s.browserIndex].Title
	}
	return view
}

func (s *server) applyBrowserEntryLocked(entry browserEntry) {
	s.last = &commandResult{
		Title:   entry.Title,
		Args:    append([]string(nil), entry.Args...),
		Output:  trimOutput(entry.Output),
		Success: entry.Success,
	}
	s.browserCurrent = entry.Target
}

func (s *server) addBookmarkLocked(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, existing := range s.browserBookmarks {
		if existing == target {
			return false
		}
	}
	s.browserBookmarks = append(s.browserBookmarks, target)
	return true
}

func buildBrowserEntry(vx6Bin, configPath, rawTarget string) (browserEntry, error) {
	target := normalizeBrowserTarget(rawTarget)
	if target == "" {
		target = "vx6://status"
	}

	title, args, canonical, err := browserTargetToArgs(target)
	if err != nil {
		return browserEntry{}, err
	}
	if vx6Bin == "" {
		return browserEntry{}, errors.New("vx6 binary is required")
	}

	out, err := runBrowserCommand(vx6Bin, configPath, args)
	return browserEntry{
		Target:    canonical,
		Title:     title,
		Args:      args,
		Output:    out,
		Success:   err == nil,
		StartedAt: time.Now(),
	}, nil
}

func runBrowserCommand(vx6Bin, configPath string, args []string) (string, error) {
	cmdOut, err := runVX6Binary(vx6Bin, configPath, args)
	if err != nil {
		return cmdOut, err
	}
	return cmdOut, nil
}

func runVX6Binary(vx6Bin, configPath string, args []string) (string, error) {
	cmd := browserCommand(vx6Bin, args)
	if configPath = strings.TrimSpace(configPath); configPath != "" {
		cmd.Env = append(cmd.Env, "VX6_CONFIG_PATH="+configPath)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out) + "\n" + err.Error(), err
	}
	return string(out), nil
}

func browserCommand(vx6Bin string, args []string) *exec.Cmd {
	cmd := exec.Command(vx6Bin, args...)
	cmd.Env = append([]string(nil), os.Environ()...)
	return cmd
}

func normalizeBrowserTarget(raw string) string {
	target := strings.TrimSpace(raw)
	target = strings.TrimPrefix(target, "vx6://")
	target = strings.TrimPrefix(target, "vx6:")
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	return target
}

func browserTargetToArgs(target string) (string, []string, string, error) {
	normalized := normalizeBrowserTarget(target)
	if normalized == "" {
		return "", nil, "", errors.New("browser target is required")
	}

	if strings.Contains(normalized, "://") {
		u, err := url.Parse(normalized)
		if err != nil {
			return "", nil, "", fmt.Errorf("parse browser target: %w", err)
		}
		if u.Scheme != "vx6" {
			return "", nil, "", fmt.Errorf("unsupported browser scheme %q", u.Scheme)
		}
		if u.Host != "" {
			normalized = u.Host + strings.TrimPrefix(u.Path, "/")
			if u.RawQuery != "" {
				normalized += "?" + u.RawQuery
			}
		} else {
			normalized = strings.TrimPrefix(u.Path, "/")
		}
	}

	parts := strings.SplitN(normalized, "/", 2)
	head := strings.ToLower(strings.TrimSpace(parts[0]))
	tail := ""
	if len(parts) > 1 {
		tail = strings.TrimSpace(parts[1])
	}

	switch head {
	case "", "home", "status":
		return "Status", []string{"status"}, "vx6://status", nil
	case "dht":
		return "DHT Status", []string{"debug", "dht-status"}, "vx6://dht", nil
	case "registry":
		return "Registry Debug", []string{"debug", "registry"}, "vx6://registry", nil
	case "services":
		return "Local Services", []string{"service"}, "vx6://services", nil
	case "peers":
		return "Local Peers", []string{"peer"}, "vx6://peers", nil
	case "identity":
		return "Identity", []string{"identity"}, "vx6://identity", nil
	case "list":
		return "List", []string{"list"}, "vx6://list", nil
	case "service":
		if tail == "" {
			return "", nil, "", errors.New("service browser target requires a service name")
		}
		return "DHT Lookup", []string{"debug", "dht-get", "--service", tail}, "vx6://service/" + tail, nil
	case "node":
		if tail == "" {
			return "", nil, "", errors.New("node browser target requires a node name")
		}
		return "DHT Lookup", []string{"debug", "dht-get", "--node", tail}, "vx6://node/" + tail, nil
	case "node-id":
		if tail == "" {
			return "", nil, "", errors.New("node-id browser target requires a node ID")
		}
		return "DHT Lookup", []string{"debug", "dht-get", "--node-id", tail}, "vx6://node-id/" + tail, nil
	case "key":
		if tail == "" {
			return "", nil, "", errors.New("key browser target requires a raw key")
		}
		return "DHT Lookup", []string{"debug", "dht-get", "--key", tail}, "vx6://key/" + tail, nil
	default:
		return "", nil, "", fmt.Errorf("unknown VX6 browser target %q", normalized)
	}
}
