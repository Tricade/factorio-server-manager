package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type issueForm struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Title       string             `yaml:"title"`
	Labels      []string           `yaml:"labels"`
	Body        []issueFormElement `yaml:"body"`
}

type issueFormElement struct {
	Type       string `yaml:"type"`
	ID         string `yaml:"id"`
	Attributes struct {
		Label       string      `yaml:"label"`
		Description string      `yaml:"description"`
		Placeholder string      `yaml:"placeholder"`
		Value       string      `yaml:"value"`
		Render      string      `yaml:"render"`
		Multiple    bool        `yaml:"multiple"`
		Options     []yaml.Node `yaml:"options"`
	} `yaml:"attributes"`
	Validations struct {
		Required bool `yaml:"required"`
	} `yaml:"validations"`
}

type issueTemplateConfig struct {
	BlankIssuesEnabled *bool       `yaml:"blank_issues_enabled"`
	ContactLinks       []yaml.Node `yaml:"contact_links"`
}

var issueFormIDPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

func TestGitHubIssueFormsStayActionable(t *testing.T) {
	tests := []struct {
		filename       string
		title          string
		label          string
		requiredFields []string
	}{
		{
			filename: "bug_report.yml", title: "[Bug]: ", label: "bug",
			requiredFields: []string{
				"preflight", "manager_version", "deployment", "host_environment", "persistence_layout",
				"factorio_version", "game_mode", "current_behavior", "expected_behavior", "reproduction",
				"logs", "configuration", "safety_confirmation",
			},
		},
		{
			filename: "feature_request.yml", title: "[Feature]: ", label: "enhancement",
			requiredFields: []string{
				"preflight", "problem", "proposal", "workflow", "area", "alternatives", "acceptance_criteria",
				"compatibility", "security_privacy", "deployment_documentation", "safety_confirmation",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			form := readIssueForm(t, test.filename)
			if len(form.Name) < 4 || strings.TrimSpace(form.Description) == "" {
				t.Fatalf("issue form must have a useful name and description: %#v", form)
			}
			if form.Title != test.title {
				t.Fatalf("unexpected title prefix %q", form.Title)
			}
			if len(form.Labels) != 1 || form.Labels[0] != test.label {
				t.Fatalf("issue form must apply the existing %q label: %#v", test.label, form.Labels)
			}

			fields := validateIssueFormElements(t, form.Body)
			for _, id := range test.requiredFields {
				field, ok := fields[id]
				if !ok {
					t.Errorf("required field %q is missing", id)
					continue
				}
				if !issueElementIsRequired(t, field) {
					t.Errorf("field %q must require a response", id)
				}
			}

			confirmation := fields["safety_confirmation"]
			if !strings.Contains(strings.ToLower(checkboxOptionLabels(t, confirmation)), "credentials") {
				t.Error("sensitive-data confirmation must mention credentials")
			}
			if test.filename == "bug_report.yml" {
				logs := fields["logs"]
				if logs.Type != "textarea" || logs.Attributes.Render != "shell" {
					t.Error("bug logs must be a shell-formatted textarea")
				}
				if !strings.Contains(logs.Attributes.Description, "30-100 lines") || !strings.Contains(logs.Attributes.Description, "not only") {
					t.Error("bug logs must request a surrounding multi-line window")
				}
			}
		})
	}
}

func TestGitHubIssueChooserRequiresAForm(t *testing.T) {
	contents := readRepositoryFile(t, ".github", "ISSUE_TEMPLATE", "config.yml")
	var config issueTemplateConfig
	decodeKnownYAML(t, contents, &config)
	if config.BlankIssuesEnabled == nil || *config.BlankIssuesEnabled {
		t.Fatal("blank public issues must remain disabled")
	}
	if config.ContactLinks == nil {
		t.Fatal("contact_links must be an explicit list")
	}
}

func TestPullRequestTemplateKeepsSafetyAndVerificationSections(t *testing.T) {
	contents := string(readRepositoryFile(t, ".github", "pull_request_template.md"))
	for _, heading := range []string{
		"## Summary", "## Failure mode or motivation", "## Safety and compatibility",
		"## Verification", "## Documentation and release",
	} {
		if !strings.Contains(contents, heading) {
			t.Errorf("pull-request template is missing %q", heading)
		}
	}
}

func readIssueForm(t *testing.T, filename string) issueForm {
	t.Helper()
	contents := readRepositoryFile(t, ".github", "ISSUE_TEMPLATE", filename)
	var form issueForm
	decodeKnownYAML(t, contents, &form)
	return form
}

func readRepositoryFile(t *testing.T, elements ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{".."}, elements...)...)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return contents
}

func decodeKnownYAML(t *testing.T, contents []byte, destination interface{}) {
	t.Helper()
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(destination); err != nil {
		t.Fatalf("decode YAML: %v", err)
	}
}

func validateIssueFormElements(t *testing.T, elements []issueFormElement) map[string]issueFormElement {
	t.Helper()
	allowedTypes := map[string]bool{"markdown": true, "input": true, "textarea": true, "dropdown": true, "checkboxes": true}
	fields := make(map[string]issueFormElement)
	for _, element := range elements {
		if !allowedTypes[element.Type] {
			t.Errorf("unsupported issue-form element type %q", element.Type)
		}
		if element.Type == "markdown" {
			if strings.TrimSpace(element.Attributes.Value) == "" {
				t.Error("markdown guidance must not be empty")
			}
			continue
		}
		if !issueFormIDPattern.MatchString(element.ID) {
			t.Errorf("invalid or missing field id %q", element.ID)
			continue
		}
		if _, duplicate := fields[element.ID]; duplicate {
			t.Errorf("duplicate field id %q", element.ID)
		}
		fields[element.ID] = element
		if strings.TrimSpace(element.Attributes.Label) == "" {
			t.Errorf("field %q must have a label", element.ID)
		}
		if (element.Type == "dropdown" || element.Type == "checkboxes") && len(element.Attributes.Options) == 0 {
			t.Errorf("field %q must provide options", element.ID)
		}
	}
	return fields
}

func issueElementIsRequired(t *testing.T, element issueFormElement) bool {
	t.Helper()
	if element.Type != "checkboxes" {
		return element.Validations.Required
	}
	if len(element.Attributes.Options) == 0 {
		return false
	}
	for _, optionNode := range element.Attributes.Options {
		var option struct {
			Label    string `yaml:"label"`
			Required bool   `yaml:"required"`
		}
		if err := optionNode.Decode(&option); err != nil {
			t.Fatalf("decode checkbox option: %v", err)
		}
		if strings.TrimSpace(option.Label) == "" || !option.Required {
			return false
		}
	}
	return true
}

func checkboxOptionLabels(t *testing.T, element issueFormElement) string {
	t.Helper()
	labels := make([]string, 0, len(element.Attributes.Options))
	for _, optionNode := range element.Attributes.Options {
		var option struct {
			Label string `yaml:"label"`
		}
		if err := optionNode.Decode(&option); err != nil {
			t.Fatalf("decode checkbox option: %v", err)
		}
		labels = append(labels, option.Label)
	}
	return strings.Join(labels, " ")
}
