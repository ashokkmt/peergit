package repository

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	pblRepositoryLimit = 50
	pblRepositoryKiB   = 250 * 1024
	pblMaxPushBytes    = 25 << 20
	pblReceiptTag      = "peergit-checkpoint-"
)

type ForgeConfig struct {
	URL, Owner, Token, WebhookURL, WebhookSecret string
}

type forgeRepository struct {
	ID            int64  `json:"id"`
	Private       bool   `json:"private"`
	Size          int64  `json:"size"`
	DefaultBranch string `json:"default_branch"`
}

type forgeError int

func (e forgeError) Error() string { return "Forgejo returned status " + strconv.Itoa(int(e)) }

func (f ForgeConfig) api(ctx context.Context, method, path string, input, output any) error {
	return f.apiAs(ctx, method, path, "", input, output)
}

func (f ForgeConfig) apiAs(ctx context.Context, method, path, username string, input, output any) error {
	res, err := f.callAs(ctx, method, path, username, input)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if output == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(output)
}

func (f ForgeConfig) call(ctx context.Context, method, path string, input any) (*http.Response, error) {
	return f.callAs(ctx, method, path, "", input)
}

func (f ForgeConfig) callAs(ctx context.Context, method, path, username string, input any) (*http.Response, error) {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(f.URL, "/")+"/api/v1"+path, body)
	if err != nil {
		return nil, errors.New("could not build Forgejo request")
	}
	req.Header.Set("Authorization", "token "+f.Token)
	if username != "" {
		req.Header.Set("Sudo", username)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("could not reach Forgejo")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		return nil, forgeError(res.StatusCode)
	}
	return res, nil
}

func (f ForgeConfig) repositoryPath(name string) string { return "/repos/" + f.Owner + "/" + name }

func (f ForgeConfig) gitURL(name string) string {
	return strings.TrimRight(f.URL, "/") + "/" + f.Owner + "/" + name + ".git"
}

func gitRefs(ctx context.Context, remote, username, secret string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + secret))
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--symref", remote)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=http.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: Basic "+auth,
		"GIT_CONFIG_KEY_1=http.followRedirects", "GIT_CONFIG_VALUE_1=false")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(pipe, (1<<20)+1))
	if readErr != nil || len(raw) > 1<<20 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, errors.New("ref manifest exceeds the pilot limit")
	}
	if err = cmd.Wait(); err != nil {
		return nil, errors.New("could not verify Git reference manifest")
	}
	refs := make(map[string]string)
	for line := range strings.Lines(string(raw)) {
		oid, ref, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "\t")
		if !ok {
			continue
		}
		if ref == "HEAD" && strings.HasPrefix(oid, "ref: refs/heads/") ||
			(strings.HasPrefix(ref, "refs/heads/") || strings.HasPrefix(ref, "refs/tags/")) && regexpCommitSHA(oid) {
			refs[ref] = oid
		}
	}
	return refs, nil
}

func (f ForgeConfig) configureRepository(ctx context.Context, name, branch string) error {
	if branch != "" {
		if err := f.protectBranch(ctx, name, branch); err != nil {
			return fmt.Errorf("protect PBL default branch: %w", err)
		}
	}
	return f.api(ctx, http.MethodPost, f.repositoryPath(name)+"/hooks", map[string]any{
		"type": "forgejo", "active": true, "events": []string{"push"},
		"config": map[string]string{"url": f.WebhookURL, "content_type": "json", "secret": f.WebhookSecret},
	}, nil)
}

func (f ForgeConfig) protectBranch(ctx context.Context, name, branch string) error {
	return f.api(ctx, http.MethodPost, f.repositoryPath(name)+"/branch_protections", map[string]any{
		"rule_name": branch, "enable_push": false, "enable_force_push": false, "required_approvals": 1,
		"dismiss_stale_approvals": true, "block_on_rejected_reviews": true, "apply_to_admins": true,
	}, nil)
}
