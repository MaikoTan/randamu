package pixiv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveRefreshToken looks up the refresh token in priority order:
// explicit file, PIXIV_REFRESH_TOKEN, systemd credential, state directory.
func ResolveRefreshToken(explicitPath, stateDir string) (string, error) {
	if explicitPath != "" {
		return readTokenFile(explicitPath)
	}
	if v := strings.TrimSpace(os.Getenv("PIXIV_REFRESH_TOKEN")); v != "" {
		return v, nil
	}
	if dir := os.Getenv("CREDENTIALS_DIRECTORY"); dir != "" {
		if tok, err := readTokenFile(filepath.Join(dir, "pixiv_refresh_token")); err == nil {
			return tok, nil
		}
	}
	if stateDir != "" {
		if tok, err := readTokenFile(filepath.Join(stateDir, "refresh_token")); err == nil {
			return tok, nil
		}
	}
	return "", fmt.Errorf("no pixiv refresh token found: set PIXIV_REFRESH_TOKEN, " +
		"pass --refresh-token-file, or inject one via systemd LoadCredential")
}

func readTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}
	return token, nil
}
