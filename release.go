package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v53/github"
	"golang.org/x/mod/modfile"
)

type releaseRunner struct {
	checkoutDir     string
	ref             string
	githubToken     string
	createTag       bool
	createRelease   bool
	tagPrefix       string
	initialTag      string
	prereleaseHook  string
	postreleaseHook string
	goModFiles      []string
	repo            string
	pushRemote      string
	tempDir         string
	githubClient    wrapper
}

func (o *releaseRunner) releaseNotesFile() string {
	return filepath.Join(o.tempDir, "release-notes")
}

func (o *releaseRunner) releaseTargetFile() string {
	return filepath.Join(o.tempDir, "release-target")
}

var modVersionRe = regexp.MustCompile(`v\d+$`)

type releaseResult struct {
	PreviousRef          string      `json:"previous-ref"`
	PreviousVersion      string      `json:"previous-version"`
	FirstRelease         bool        `json:"first-release"`
	ReleaseVersion       string      `json:"release-version,omitempty"`
	ReleaseTag           string      `json:"release-tag,omitempty"`
	ChangeLevel          changeLevel `json:"change-level"`
	CreatedTag           bool        `json:"created-tag,omitempty"`
	CreatedRelease       bool        `json:"created-release,omitempty"`
	PrereleaseHookOutput string      `json:"prerelease-hook-output"`
}

func (o *releaseRunner) next(ctx context.Context) (*releaseResult, error) {
	// allows any semver that doesn't have a prerelease or build metadata
	stableConstraint, err := semver.NewConstraint("*")
	if err != nil {
		return nil, err
	}
	prevOpts := prevVersionOptions{
		head:        o.ref,
		repoDir:     o.checkoutDir,
		prefixes:    []string{o.tagPrefix},
		constraints: stableConstraint,
	}
	prevRef, err := getPrevTag(ctx, &prevOpts)
	if err != nil {
		return nil, err
	}
	firstRelease := prevRef == ""
	if firstRelease {
		return &releaseResult{
			FirstRelease:   true,
			ReleaseTag:     o.initialTag,
			ReleaseVersion: strings.TrimPrefix(o.initialTag, o.tagPrefix),
			ChangeLevel:    changeLevelNoChange,
		}, nil
	}
	prevVersion := strings.TrimPrefix(prevRef, o.tagPrefix)
	result := releaseResult{
		PreviousRef:     prevRef,
		PreviousVersion: prevVersion,
	}
	var nextRes *nextResult
	nextRes, err = getNext(ctx, &nextOptions{
		repo:        o.repo,
		gh:          o.githubClient,
		prevVersion: prevVersion,
		base:        prevRef,
		head:        o.ref,
	})
	if err != nil {
		return nil, err
	}
	result.ReleaseVersion = nextRes.NextVersion
	result.ReleaseTag = o.tagPrefix + nextRes.NextVersion
	result.ChangeLevel = nextRes.ChangeLevel
	return &result, nil
}

func (o *releaseRunner) runGoValidation(modFile string, result *releaseResult) error {
	mfPath := filepath.Join(o.checkoutDir, filepath.FromSlash(modFile))
	content, err := os.ReadFile(mfPath)
	if err != nil {
		return err
	}
	mf, err := modfile.ParseLax(mfPath, content, nil)
	if err != nil {
		return err
	}
	sv, err := semver.NewVersion(result.ReleaseVersion)
	if err != nil {
		return err
	}
	major := int(sv.Major())
	wantM := ""
	if major > 1 {
		wantM = fmt.Sprintf("v%d", major)
	}
	m := modVersionRe.FindString(mf.Module.Mod.Path)
	if m != wantM {
		return fmt.Errorf("module %s has version suffix %q, want %q", mf.Module.Mod.Path, m, wantM)
	}
	return nil
}

func (o *releaseRunner) repoOwner() string {
	return strings.SplitN(o.repo, "/", 2)[0]
}

func (o *releaseRunner) repoName() string {
	return strings.SplitN(o.repo, "/", 2)[1]
}

func (o *releaseRunner) getReleaseTarget() (string, error) {
	targetFile := o.releaseTargetFile()
	targetInfo, err := os.Stat(targetFile)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	target := ""
	if err == nil && !targetInfo.IsDir() {
		content, e := os.ReadFile(o.releaseTargetFile())
		if e != nil {
			return "", e
		}
		target = strings.TrimSpace(string(content))
	}
	if target == "" {
		return o.ref, nil
	}
	return target, nil
}

func (o *releaseRunner) getReleaseNotes(ctx context.Context, result *releaseResult) (string, error) {
	notesInfo, err := os.Stat(o.releaseNotesFile())
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err == nil && !notesInfo.IsDir() {
		content, e := os.ReadFile(o.releaseNotesFile())
		if e != nil {
			return "", e
		}
		return string(content), nil
	}
	// first release is empty by default
	if result.FirstRelease {
		return "", nil
	}
	return o.githubClient.GenerateReleaseNotes(ctx, o.repoOwner(), o.repoName(), &github.GenerateNotesOptions{
		TagName:         result.ReleaseTag,
		PreviousTagName: &result.PreviousRef,
	})
}

func (o *releaseRunner) run(ctx context.Context) (*releaseResult, error) {
	createTag := o.createTag
	if o.createRelease {
		createTag = true
	}
	shallow, err := runCmd(o.checkoutDir, nil, "git", "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, err
	}
	if shallow == "true" {
		return nil, fmt.Errorf("shallow clones are not supported")
	}
	result, err := o.next(ctx)
	if err != nil {
		return nil, err
	}
	if result.ReleaseVersion == "" || !createTag {
		return result, nil
	}
	if !result.FirstRelease && result.ChangeLevel == changeLevelNoChange {
		return result, nil
	}

	runEnv := map[string]string{
		"RELEASE_VERSION":    result.ReleaseVersion,
		"RELEASE_TAG":        result.ReleaseTag,
		"PREVIOUS_VERSION":   result.PreviousVersion,
		"FIRST_RELEASE":      fmt.Sprintf("%t", result.FirstRelease),
		"GITHUB_TOKEN":       o.githubToken,
		"RELEASE_NOTES_FILE": o.releaseNotesFile(),
		"RELEASE_TARGET":     o.releaseTargetFile(),
	}

	prereleaseOut, abort, err := runPrereleaseHook(o.checkoutDir, runEnv, o.prereleaseHook)
	if err != nil {
		return nil, err
	}
	result.PrereleaseHookOutput = prereleaseOut
	if abort {
		return result, nil
	}

	for _, mf := range o.goModFiles {
		err = o.runGoValidation(mf, result)
		if err != nil {
			return nil, err
		}
	}

	target, err := o.getReleaseTarget()
	if err != nil {
		return nil, err
	}

	_, err = runCmd(o.checkoutDir, nil, "git", "tag", result.ReleaseTag, target)
	if err != nil {
		return nil, err
	}

	_, err = runCmd(o.checkoutDir, nil, "git", "push", o.pushRemote, result.ReleaseTag)
	if err != nil {
		return nil, err
	}

	result.CreatedTag = true

	if !o.createRelease {
		return result, nil
	}

	releaseNotes, err := o.getReleaseNotes(ctx, result)
	if err != nil {
		return nil, err
	}

	err = o.githubClient.CreateRelease(ctx, o.repoOwner(), o.repoName(), &github.RepositoryRelease{
		TagName:    &result.ReleaseTag,
		Name:       &result.ReleaseTag,
		Body:       &releaseNotes,
		MakeLatest: github.String("legacy"),
	})
	if err != nil {
		return nil, err
	}

	result.CreatedRelease = true

	if o.postreleaseHook != "" {
		_, err = runCmd(o.checkoutDir, runEnv, "sh", "-c", o.postreleaseHook)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func runPrereleaseHook(dir string, env map[string]string, hook string) (stdout string, abort bool, _ error) {
	if hook == "" {
		return "", false, nil
	}
	var stdoutBuf bytes.Buffer
	cmd := exec.Command("sh", "-c", hook)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Stdout = &stdoutBuf
	err := cmd.Run()
	if err != nil {
		exitErr := asExitErr(err)
		if exitErr != nil {
			err = errors.Join(err, errors.New(string(exitErr.Stderr)))
			if exitErr.ExitCode() == 10 {
				return stdoutBuf.String(), true, nil
			}
		}
		return "", false, err
	}
	return stdoutBuf.String(), false, nil
}

func runCmd(dir string, env map[string]string, command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	out, err := cmd.Output()
	if err != nil {
		exitErr := asExitErr(err)
		if exitErr != nil {
			err = errors.Join(err, errors.New(string(exitErr.Stderr)))
		}
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

func asExitErr(err error) *exec.ExitError {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	return nil
}
