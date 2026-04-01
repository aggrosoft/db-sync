package secrets

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var validNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

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
