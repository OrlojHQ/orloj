package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const fallbackServer = "http://127.0.0.1:8080"

// resolvedCliConfig is set at the start of each Run invocation after loading config from disk.
var resolvedCliConfig *orlojctlConfig

// orlojctlConfig is stored at UserConfigDir/orlojctl/config.json (mode 0600).
type orlojctlConfig struct {
	CurrentProfile string                  `json:"current_profile,omitempty"`
	Profiles       map[string]profileEntry `json:"profiles,omitempty"`
}

type profileEntry struct {
	Server   string `json:"server,omitempty"`
	Token    string `json:"token,omitempty"`
	TokenEnv string `json:"token_env,omitempty"`
}

func orlojctlConfigPath() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "orlojctl", "config.json"), nil
}

func loadOrlojctlConfig() (*orlojctlConfig, error) {
	path, err := orlojctlConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &orlojctlConfig{Profiles: map[string]profileEntry{}}, nil
		}
		return nil, err
	}
	var c orlojctlConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Profiles == nil {
		c.Profiles = map[string]profileEntry{}
	}
	return &c, nil
}

func saveOrlojctlConfig(c *orlojctlConfig) error {
	if c == nil {
		return errors.New("nil config")
	}
	if c.Profiles == nil {
		c.Profiles = map[string]profileEntry{}
	}
	path, err := orlojctlConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// defaultServerResolved returns the default --server value when the flag is omitted.
// Precedence: ORLOJCTL_SERVER > ORLOJ_SERVER > active profile server > fallbackServer.
func defaultServerResolved(cfg *orlojctlConfig) string {
	if v := strings.TrimSpace(os.Getenv("ORLOJCTL_SERVER")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("ORLOJ_SERVER")); v != "" {
		return v
	}
	if cfg != nil {
		if pe, ok := cfg.activeProfile(); ok && strings.TrimSpace(pe.Server) != "" {
			return strings.TrimSpace(pe.Server)
		}
	}
	return fallbackServer
}

func (c *orlojctlConfig) activeProfile() (profileEntry, bool) {
	if c == nil || strings.TrimSpace(c.CurrentProfile) == "" {
		return profileEntry{}, false
	}
	name := strings.TrimSpace(c.CurrentProfile)
	pe, ok := c.Profiles[name]
	return pe, ok
}

func tokenFromProfile(cfg *orlojctlConfig) string {
	if cfg == nil {
		return ""
	}
	pe, ok := cfg.activeProfile()
	if !ok {
		return ""
	}
	if t := strings.TrimSpace(pe.Token); t != "" {
		return t
	}
	if e := strings.TrimSpace(pe.TokenEnv); e != "" {
		return strings.TrimSpace(os.Getenv(e))
	}
	return ""
}

func runConfig(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: orlojctl config path|get|use <name>|set-profile <name> [--server URL] [--token value] [--token-env NAME]")
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "path":
		p, err := orlojctlConfigPath()
		if err != nil {
			return err
		}
		fmt.Println(p)
		return nil
	case "get":
		path, err := orlojctlConfigPath()
		if err != nil {
			return err
		}
		_, statErr := os.Stat(path)
		if os.IsNotExist(statErr) {
			fmt.Printf("config: %s\n", path)
			fmt.Println("no config file yet (normal on first use). Run `orlojctl config set-profile <name> --server URL [--token-env VAR]` to create it.")
			return nil
		}
		if statErr != nil {
			return statErr
		}
		cfg, err := loadOrlojctlConfig()
		if err != nil {
			return err
		}
		fmt.Printf("config: %s\n", path)
		fmt.Printf("current_profile: %q\n", cfg.CurrentProfile)
		if cfg.CurrentProfile != "" {
			if pe, ok := cfg.Profiles[cfg.CurrentProfile]; ok {
				fmt.Printf("  server: %s\n", pe.Server)
				if strings.TrimSpace(pe.Token) != "" {
					fmt.Printf("  token: (set)\n")
				} else {
					fmt.Printf("  token: (not set)\n")
				}
				if strings.TrimSpace(pe.TokenEnv) != "" {
					fmt.Printf("  token_env: %s\n", pe.TokenEnv)
				}
			} else {
				fmt.Printf("  (profile %q not found in profiles)\n", cfg.CurrentProfile)
			}
		}
		if len(cfg.Profiles) > 0 {
			fmt.Println("profiles:")
			names := make([]string, 0, len(cfg.Profiles))
			for name := range cfg.Profiles {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Printf("  - %s\n", name)
			}
		}
		return nil
	case "use":
		if len(args) < 2 {
			return errors.New("usage: orlojctl config use <name>")
		}
		name := strings.TrimSpace(args[1])
		if name == "" {
			return errors.New("profile name is required")
		}
		cfg, err := loadOrlojctlConfig()
		if err != nil {
			return err
		}
		if _, ok := cfg.Profiles[name]; !ok {
			return fmt.Errorf("unknown profile %q (use config set-profile first)", name)
		}
		cfg.CurrentProfile = name
		if err := saveOrlojctlConfig(cfg); err != nil {
			return err
		}
		resolvedCliConfig = cfg
		fmt.Printf("active profile: %s\n", name)
		return nil
	case "set-profile":
		if len(args) < 2 {
			return errors.New("usage: orlojctl config set-profile <name> [--server URL] [--token value] [--token-env NAME]")
		}
		name := strings.TrimSpace(args[1])
		if name == "" {
			return errors.New("profile name is required")
		}
		fs := flag.NewFlagSet("config set-profile", flag.ContinueOnError)
		server := fs.String("server", "", "Orloj API base URL (e.g. https://orloj.example.com)")
		token := fs.String("token", "", "bearer token (optional; prefer --token-env for secrets)")
		tokenEnv := fs.String("token-env", "", "read token from this environment variable at runtime")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		cfg, err := loadOrlojctlConfig()
		if err != nil {
			return err
		}
		pe := cfg.Profiles[name]
		if strings.TrimSpace(*server) != "" {
			pe.Server = strings.TrimRight(strings.TrimSpace(*server), "/")
		}
		if *token != "" {
			pe.Token = strings.TrimSpace(*token)
		}
		if strings.TrimSpace(*tokenEnv) != "" {
			pe.TokenEnv = strings.TrimSpace(*tokenEnv)
		}
		cfg.Profiles[name] = pe
		if cfg.CurrentProfile == "" {
			cfg.CurrentProfile = name
		}
		if err := saveOrlojctlConfig(cfg); err != nil {
			return err
		}
		resolvedCliConfig = cfg
		fmt.Printf("profile %q updated\n", name)
		return nil
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}
