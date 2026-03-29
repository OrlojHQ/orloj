package cli

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

func runValidate(args []string) error {
	fsFlags := flag.NewFlagSet("validate", flag.ContinueOnError)
	manifestPath := fsFlags.String("f", "", "path to manifest file or directory")
	if err := fsFlags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*manifestPath) == "" {
		return errors.New("-f is required: path to manifest file or directory")
	}

	files, err := manifestPaths(*manifestPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no manifest files found in %s", *manifestPath)
	}

	var errs []string
	valid := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f, err))
			continue
		}

		kind, err := resources.DetectKind(raw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f, err))
			continue
		}

		normKind, name, _, err := resources.ParseManifest(kind, raw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f, err))
			continue
		}

		fmt.Printf("  valid  %s/%s  (%s)\n", normKind, name, f)
		valid++
	}

	if len(errs) > 0 {
		fmt.Printf("\n%d valid, %d invalid:\n", valid, len(errs))
		for _, e := range errs {
			fmt.Printf("  error  %s\n", e)
		}
		return fmt.Errorf("validation failed for %d file(s)", len(errs))
	}

	fmt.Printf("\n%d file(s) valid\n", valid)
	return nil
}

func manifestPaths(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot access %s: %w", path, err)
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	var out []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".yaml", ".yml", ".json":
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", path, err)
	}
	sort.Strings(out)
	return out, nil
}
