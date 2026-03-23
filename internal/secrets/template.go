package secrets

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)\}`)
var tokenPattern = regexp.MustCompile(`\$\{([^}]+)\}`)
var validNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

type ResolvedTemplate struct {
	value        string
	placeholders []string
	missing      []string
}

func (resolved ResolvedTemplate) Value() string {
	return resolved.value
}

func (resolved ResolvedTemplate) Placeholders() []string {
	return append([]string(nil), resolved.placeholders...)
}

func (resolved ResolvedTemplate) Missing() []string {
	return append([]string(nil), resolved.missing...)
}

func (resolved ResolvedTemplate) String() string {
	return "[redacted]"
}

func ParsePlaceholders(input string) ([]string, error) {
	matches := tokenPattern.FindAllStringSubmatch(input, -1)
	seen := map[string]struct{}{}
	placeholders := make([]string, 0, len(matches))
	for _, match := range matches {
		name := match[1]
		if !validNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid placeholder %q", match[0])
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		placeholders = append(placeholders, name)
	}
	for _, match := range placeholderPattern.FindAllStringSubmatch(input, -1) {
		_ = match
	}
	sort.Strings(placeholders)
	return placeholders, nil
}

func ResolveTemplate(input string, env map[string]string) (ResolvedTemplate, error) {
	placeholders, err := ParsePlaceholders(input)
	if err != nil {
		return ResolvedTemplate{}, err
	}
	missing := make([]string, 0)
	resolved := tokenPattern.ReplaceAllStringFunc(input, func(token string) string {
		name := tokenPattern.FindStringSubmatch(token)[1]
		value, ok := env[name]
		if !ok || value == "" {
			missing = append(missing, name)
			return token
		}
		return value
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		return ResolvedTemplate{placeholders: placeholders, missing: missing}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return ResolvedTemplate{value: resolved, placeholders: placeholders}, nil
}

func ValidateTemplatePolicy(input string) error {
	placeholders, err := ParsePlaceholders(input)
	if err != nil {
		return err
	}
	if len(placeholders) == 0 {
		return fmt.Errorf("dsn template must include at least one ${NAME} placeholder for secret values")
	}
	return nil
}

func EnvReference(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return "${" + name + "}"
}

func RedactedValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "[redacted]"
}

func CurrentEnvironment() map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		env[parts[0]] = parts[1]
	}
	return env
}

func LoadEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	env := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid env assignment %q", line)
		}
		key := strings.TrimSpace(parts[0])
		if !validNamePattern.MatchString(key) {
			return nil, fmt.Errorf("invalid env key %q", key)
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		env[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return env, nil
}
