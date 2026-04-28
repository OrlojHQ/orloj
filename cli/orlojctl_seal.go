package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	yaml "go.yaml.in/yaml/v2"

	"github.com/OrlojHQ/orloj/resources"
)

func newSealCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seal",
		Short: "Work with SealedSecret public keys and manifests",
	}
	cmd.AddCommand(newSealPublicKeyCommand(), newSealSecretCommand())
	return cmd
}

func newSealPublicKeyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "public-key",
		Short: "Fetch the active SealedSecret public key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			server := resolveServer(cmd)
			payload, err := fetchSealingPublicKey(server)
			if err != nil {
				return err
			}
			fmt.Print(payload.PublicKeyPEM)
			return nil
		},
	}
}

func newSealSecretCommand() *cobra.Command {
	var file string
	var out string
	var format string
	var stdout bool
	var literals stringSliceFlag
	cmd := &cobra.Command{
		Use:   "secret [name]",
		Short: "Seal a Secret manifest into a SealedSecret manifest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := normalizeSealOutputFormat(format)
			if err != nil {
				return err
			}
			if stdout && strings.TrimSpace(out) != "" {
				return errors.New("use either --stdout or --out, not both")
			}

			name := ""
			if len(args) > 0 {
				name = strings.TrimSpace(args[0])
			}
			secret, err := resolveSealSecretInput(cmd, name, file, literals)
			if err != nil {
				return err
			}

			server := resolveServer(cmd)
			payload, err := fetchSealingPublicKey(server)
			if err != nil {
				return err
			}
			publicKey, keyID, err := resources.ParseSealingPublicKeyPEM(payload.PublicKeyPEM)
			if err != nil {
				return err
			}
			if strings.TrimSpace(payload.KeyID) != "" {
				keyID = strings.TrimSpace(payload.KeyID)
			}
			sealed, err := resources.SealSecret(secret, keyID, publicKey)
			if err != nil {
				return err
			}

			// Status is server-side state set on apply; omit it from
			// the CLI output so sealed manifests are clean for VCS.
			sealed.Status = resources.SealedSecretStatus{}

			body, err := marshalSealOutput(sealed, format)
			if err != nil {
				return err
			}

			if stdout {
				fmt.Print(string(body))
				return nil
			}

			target := strings.TrimSpace(out)
			if target == "" {
				target = defaultSealedSecretOutputPath(file, secret.Metadata.Name, format)
			}
			if err := writeSealedSecretOutput(target, body); err != nil {
				return err
			}
			if rel, err := filepath.Rel(".", target); err == nil && !strings.HasPrefix(rel, "..") {
				target = rel
			}
			fmt.Printf("wrote %s\n", target)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to a Secret manifest")
	cmd.Flags().StringVarP(&out, "out", "o", "", "Write the generated SealedSecret manifest to this path")
	cmd.Flags().StringVar(&format, "format", "yaml", "Output format: yaml|json")
	cmd.Flags().BoolVar(&stdout, "stdout", false, "Print the generated SealedSecret manifest to stdout instead of writing a file")
	cmd.Flags().Var(&literals, "from-literal", "key=value pair (repeatable)")
	return cmd
}

type sealingPublicKeyResponse struct {
	KeyID        string `json:"keyId"`
	Algorithm    string `json:"algorithm"`
	PublicKeyPEM string `json:"publicKeyPEM"`
}

func fetchSealingPublicKey(server string) (sealingPublicKeyResponse, error) {
	resp, err := http.Get(strings.TrimRight(server, "/") + "/v1/sealing-key/public")
	if err != nil {
		return sealingPublicKeyResponse{}, fmt.Errorf("fetch sealing public key failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return sealingPublicKeyResponse{}, fmt.Errorf("fetch sealing public key failed: %s", strings.TrimSpace(string(body)))
	}
	var payload sealingPublicKeyResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return sealingPublicKeyResponse{}, fmt.Errorf("decode sealing public key response failed: %w", err)
	}
	return payload, nil
}

func resolveSealSecretInput(cmd *cobra.Command, name, file string, literals []string) (resources.Secret, error) {
	file = strings.TrimSpace(file)
	name = strings.TrimSpace(name)
	overrideNamespace := resolveNamespace(cmd)

	switch {
	case file != "" && len(literals) > 0:
		return resources.Secret{}, errors.New("use either --file or --from-literal, not both")
	case file != "" && name != "":
		return resources.Secret{}, errors.New("secret name argument cannot be used with --file")
	case file == "" && len(literals) == 0:
		return resources.Secret{}, errors.New("provide either --file or at least one --from-literal")
	}

	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return resources.Secret{}, fmt.Errorf("read manifest failed: %w", err)
		}
		kind, err := resources.DetectKind(raw)
		if err != nil {
			return resources.Secret{}, err
		}
		if !strings.EqualFold(strings.TrimSpace(kind), "Secret") {
			return resources.Secret{}, fmt.Errorf("seal secret requires kind Secret, got %q", kind)
		}
		secret, err := resources.ParseSecretManifest(raw)
		if err != nil {
			return resources.Secret{}, err
		}
		if overrideNamespace != "" {
			secret.Metadata.Namespace = overrideNamespace
			if err := secret.Normalize(); err != nil {
				return resources.Secret{}, fmt.Errorf("invalid secret: %w", err)
			}
		}
		return secret, nil
	}

	if name == "" {
		return resources.Secret{}, errors.New("secret name is required when sealing from --from-literal")
	}
	stringData, err := parseSealLiteralPairs(literals)
	if err != nil {
		return resources.Secret{}, err
	}
	secret := resources.Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: resources.ObjectMeta{
			Name:      name,
			Namespace: overrideNamespace,
		},
		Spec: resources.SecretSpec{
			StringData: stringData,
		},
	}
	if err := secret.Normalize(); err != nil {
		return resources.Secret{}, fmt.Errorf("invalid secret: %w", err)
	}
	return secret, nil
}

func parseSealLiteralPairs(literals []string) (map[string]string, error) {
	if len(literals) == 0 {
		return nil, errors.New("at least one --from-literal key=value is required")
	}
	stringData := make(map[string]string, len(literals))
	for _, lit := range literals {
		parts := strings.SplitN(lit, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid --from-literal %q: expected key=value", lit)
		}
		stringData[strings.TrimSpace(parts[0])] = parts[1]
	}
	return stringData, nil
}

func normalizeSealOutputFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "yaml", "yml":
		return "yaml", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported --format %q (expected yaml or json)", raw)
	}
}

func marshalSealOutput(obj any, format string) ([]byte, error) {
	switch format {
	case "json":
		out, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal sealed secret failed: %w", err)
		}
		return append(out, '\n'), nil
	case "yaml":
		rawJSON, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("marshal sealed secret failed: %w", err)
		}
		var generic any
		if err := json.Unmarshal(rawJSON, &generic); err != nil {
			return nil, fmt.Errorf("decode sealed secret failed: %w", err)
		}
		out, err := yaml.Marshal(generic)
		if err != nil {
			return nil, fmt.Errorf("marshal sealed secret failed: %w", err)
		}
		if len(out) == 0 || out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported seal output format %q", format)
	}
}

func defaultSealedSecretOutputPath(inputFile, name, format string) string {
	ext := ".yaml"
	if format == "json" {
		ext = ".json"
	}
	if strings.TrimSpace(inputFile) != "" {
		dir := filepath.Dir(inputFile)
		base := filepath.Base(inputFile)
		currentExt := strings.ToLower(filepath.Ext(base))
		switch currentExt {
		case ".yaml", ".yml", ".json":
			base = strings.TrimSuffix(base, filepath.Ext(base))
		}
		return filepath.Join(dir, base+".sealed"+ext)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "sealed-secret"
	}
	return name + ".sealed" + ext
}

func writeSealedSecretOutput(target string, body []byte) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return errors.New("output path cannot be empty")
	}
	dir := filepath.Dir(target)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output directory failed: %w", err)
		}
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return fmt.Errorf("write sealed secret failed: %w", err)
	}
	return nil
}
