package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a podfile.yaml for the current project",
	Long: `Detect the project type and generate a starter podfile.yaml.

  podspawn init                -> auto-detect and scaffold
  podspawn init --template go  -> use specific template
  podspawn init -y             -> accept defaults, no prompts`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	templateName, _ := cmd.Flags().GetString("template")
	yes, _ := cmd.Flags().GetBool("yes")
	update, _ := cmd.Flags().GetBool("update")

	if update {
		fmt.Fprintf(os.Stderr, "Fetching latest templates from podspawn/podfiles...\n")
		if err := podfile.UpdateTemplateCache(); err != nil {
			return fmt.Errorf("updating templates: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Templates updated.\n")
	}

	outPath := filepath.Join(cwd, "podfile.yaml")
	if _, err := os.Stat(outPath); err == nil {
		if !yes {
			fmt.Print("podfile.yaml already exists. Overwrite? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(answer)) != "y" {
				return fmt.Errorf("aborted")
			}
		}
	}

	// Detect or use explicit template
	var detectedMarker string
	if templateName == "" {
		templateName, detectedMarker = podfile.DetectProjectType(cwd)
		if detectedMarker != "" {
			fmt.Fprintf(os.Stderr, "Detected: %s project (%s found)\n", templateName, detectedMarker)
		} else {
			fmt.Fprintf(os.Stderr, "No project markers found, using minimal template\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "Using template: %s\n", templateName)
	}

	// Load template
	data, err := podfile.LookupTemplate(templateName)
	if err != nil {
		return fmt.Errorf("loading template %q: %w", templateName, err)
	}

	if !yes {
		data, err = runInitWizard(data, templateName)
		if err != nil {
			return err
		}
	}

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("writing podfile.yaml: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Created podfile.yaml\n\nRun 'podspawn dev' to start.\n")
	return nil
}

func runInitWizard(templateData []byte, templateName string) ([]byte, error) {
	reader := bufio.NewReader(os.Stdin)

	rawPf, err := podfile.ParseRaw(strings.NewReader(string(templateData)))
	if err != nil {
		return templateData, nil
	}
	pf := &rawPf.Podfile

	// Base image
	fmt.Fprintf(os.Stderr, "Base image? [%s] ", pf.Base)
	if answer, _ := reader.ReadString('\n'); strings.TrimSpace(answer) != "" {
		pf.Base = strings.TrimSpace(answer)
	}

	// Packages
	pkgStr := strings.Join(pf.Packages, ", ")
	fmt.Fprintf(os.Stderr, "Packages? [%s] ", pkgStr)
	if answer, _ := reader.ReadString('\n'); strings.TrimSpace(answer) != "" {
		pf.Packages = splitCSV(strings.TrimSpace(answer))
	}

	// Services
	fmt.Fprintf(os.Stderr, "Add services? (postgres/redis/none) [none] ")
	if answer, _ := reader.ReadString('\n'); strings.TrimSpace(answer) != "" {
		svcAnswer := strings.TrimSpace(strings.ToLower(answer))
		if svcAnswer != "none" {
			for _, svc := range splitCSV(svcAnswer) {
				switch svc {
				case "postgres":
					pf.Services = append(pf.Services, podfile.ServiceConfig{
						Name:  "postgres",
						Image: "postgres:16",
						Ports: []int{5432},
						Env:   map[string]string{"POSTGRES_PASSWORD": "devpass", "POSTGRES_DB": "dev"},
					})
				case "redis":
					pf.Services = append(pf.Services, podfile.ServiceConfig{
						Name:  "redis",
						Image: "redis:7",
						Ports: []int{6379},
					})
				}
			}
		}
	}

	// Ports
	portStr := ""
	for _, p := range pf.Ports.Expose {
		if portStr != "" {
			portStr += ", "
		}
		portStr += fmt.Sprintf("%d", p)
	}
	fmt.Fprintf(os.Stderr, "Expose ports? [%s] ", portStr)
	if answer, _ := reader.ReadString('\n'); strings.TrimSpace(answer) != "" {
		pf.Ports.Expose = nil
		for _, p := range splitCSV(strings.TrimSpace(answer)) {
			var port int
			if _, err := fmt.Sscanf(p, "%d", &port); err == nil {
				pf.Ports.Expose = append(pf.Ports.Expose, port)
			}
		}
	}

	// Re-serialize with extends preserved from template
	return podfile.MarshalCanonical(pf)
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func init() {
	initCmd.Flags().StringP("template", "t", "", "use specific template (go, node, python, rust, fullstack, minimal)")
	initCmd.Flags().BoolP("yes", "y", false, "accept defaults, skip wizard")
	initCmd.Flags().Bool("update", false, "fetch latest templates from podspawn/podfiles")
	rootCmd.AddCommand(initCmd)
}
