// Package prompts holds the system prompt for every pipeline stage.
//
// Prompts are plain Markdown files embedded at build time. An operator can
// override any of them without recompiling by pointing the pipeline at a
// directory that contains a file with the same name.
package prompts

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed *.md
var files embed.FS

// Stage names. They double as prompt file names.
const (
	Intake   = "intake"
	Content  = "content"
	Plan     = "plan"
	Critique = "critique"
	Build    = "build"
	Review   = "review"
	Repair   = "repair"
	Classify = "classify" // product: sorts a chat message before anything is charged
	Edit     = "edit"     // product: applies one owner request to a built page
)

// Stages lists every stage: the build pipeline in order, then the stages the
// product uses for chat edits.
var Stages = []string{Intake, Content, Plan, Critique, Build, Review, Repair, Classify, Edit}

const defaultsToken = "{{DEFAULTS}}"

// Load returns the system prompt for a stage, with the shared list of
// generic defaults expanded wherever a prompt asks for it.
func Load(stage, overrideDir string) (string, error) {
	body, err := read(stage+".md", overrideDir)
	if err != nil {
		return "", fmt.Errorf("prompt %s: %w", stage, err)
	}
	if strings.Contains(body, defaultsToken) {
		defaults, err := read("defaults.md", overrideDir)
		if err != nil {
			return "", fmt.Errorf("prompt defaults: %w", err)
		}
		body = strings.ReplaceAll(body, defaultsToken, demote(defaults))
	}
	return strings.TrimSpace(body) + "\n", nil
}

func read(name, overrideDir string) (string, error) {
	if overrideDir != "" {
		b, err := os.ReadFile(filepath.Join(overrideDir, name))
		if err == nil {
			return string(b), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}
	b, err := files.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// LoadKit returns the system prompt for a stage when the page is built with
// the Faz kit. The build stage gets its own prompt with the component
// reference; plan and critique learn what the kit can build; repair and edit
// learn to keep the page a kit page. icons lists the icon names the kit has.
func LoadKit(stage, overrideDir string, icons []string) (string, error) {
	if stage != Build {
		body, err := Load(stage, overrideDir)
		if err != nil {
			return "", err
		}
		var extra []string
		switch stage {
		case Plan, Critique:
			extra = []string{"kit-plan.md"}
		case Repair, Edit:
			extra = []string{"kit-edit.md", "kit-components.md"}
		}
		for _, name := range extra {
			b, err := read(name, overrideDir)
			if err != nil {
				return "", fmt.Errorf("prompt %s: %w", name, err)
			}
			body += "\n" + strings.ReplaceAll(strings.TrimSpace(b), iconsToken, strings.Join(icons, ", ")) + "\n"
		}
		return body, nil
	}
	body, err := read("build-kit.md", overrideDir)
	if err != nil {
		return "", fmt.Errorf("prompt build-kit: %w", err)
	}
	ref, err := read("kit-components.md", overrideDir)
	if err != nil {
		return "", fmt.Errorf("prompt kit-components: %w", err)
	}
	body = strings.ReplaceAll(body, kitToken, strings.TrimSpace(ref))
	body = strings.ReplaceAll(body, iconsToken, strings.Join(icons, ", "))
	return strings.TrimSpace(body) + "\n", nil
}

const (
	kitToken   = "{{KIT}}"
	iconsToken = "{{ICONS}}"
)

// demote pushes every Markdown heading in the shared defaults file one level
// down, so it nests under the prompt that includes it.
func demote(md string) string {
	lines := strings.Split(md, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "#") {
			lines[i] = "#" + l
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
