package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func newA2ACommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "a2a",
		Short: "A2A protocol commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newA2ACardCommand(), newA2ATestCommand())
	return cmd
}

func newA2ACardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "card <agent-system-name>",
		Short: "Preview the generated Agent Card for an AgentSystem",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := resolveServer(cmd)
			name := strings.TrimSpace(args[0])
			if name == "" {
				return errors.New("agentsystem name is required")
			}

			if ns := resolveNamespace(cmd); ns != "" && !strings.Contains(name, "/") {
				name = ns + "/" + name
			}

			cardURL := strings.TrimRight(server, "/") + "/v1/agent-systems/" + url.PathEscape(name) + "/.well-known/agent-card.json"
			resp, err := http.Get(cardURL)
			if err != nil {
				return fmt.Errorf("fetch agent card failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("fetch agent card failed (%d): %s", resp.StatusCode, bytes.TrimSpace(body))
			}

			var pretty bytes.Buffer
			if err := json.Indent(&pretty, body, "", "  "); err != nil {
				fmt.Print(string(body))
				return nil
			}
			fmt.Println(pretty.String())
			return nil
		},
	}
	return cmd
}

func newA2ATestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <remote-url>",
		Short: "Fetch a remote Agent Card and validate connectivity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rawURL := strings.TrimSpace(args[0])
			if rawURL == "" {
				return errors.New("remote URL is required")
			}

			cardURL := rawURL
			if !strings.Contains(rawURL, "/.well-known/") {
				cardURL = strings.TrimSuffix(rawURL, "/") + "/.well-known/agent-card.json"
			}

			fmt.Printf("Fetching Agent Card from %s ...\n", cardURL)

			resp, err := http.Get(cardURL)
			if err != nil {
				return fmt.Errorf("connection failed: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("remote returned %d: %s", resp.StatusCode, bytes.TrimSpace(body))
			}

			var card struct {
				Name            string `json:"name"`
				Description     string `json:"description"`
				URL             string `json:"url"`
				ProtocolVersion string `json:"protocolVersion"`
				Capabilities    struct {
					Streaming         bool `json:"streaming"`
					PushNotifications bool `json:"pushNotifications"`
				} `json:"capabilities"`
				Skills []struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"skills"`
			}

			if err := json.Unmarshal(body, &card); err != nil {
				return fmt.Errorf("invalid Agent Card JSON: %w", err)
			}

			fmt.Printf("Agent: %s\n", card.Name)
			if card.Description != "" {
				fmt.Printf("Description: %s\n", card.Description)
			}
			fmt.Printf("URL: %s\n", card.URL)
			if card.ProtocolVersion != "" {
				fmt.Printf("Protocol Version: %s\n", card.ProtocolVersion)
			}
			fmt.Printf("Streaming: %t\n", card.Capabilities.Streaming)
			fmt.Printf("Push Notifications: %t\n", card.Capabilities.PushNotifications)
			if len(card.Skills) > 0 {
				fmt.Printf("Skills (%d):\n", len(card.Skills))
				for _, skill := range card.Skills {
					desc := skill.Description
					if len(desc) > 60 {
						desc = desc[:57] + "..."
					}
					fmt.Printf("  - %s: %s\n", skill.Name, desc)
				}
			}

			fmt.Println("\nConnectivity: OK")
			return nil
		},
	}
	return cmd
}
