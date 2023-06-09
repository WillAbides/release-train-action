package internal

import (
	"context"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
)

type GithubClient interface {
	ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]Pull, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error)
	GenerateReleaseNotes(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error)
	CreateRelease(ctx context.Context, owner, repo string, release *github.RepositoryRelease) error
}

func NewGithubClient(ctx context.Context, baseUrl, token, userAgent string) (GithubClient, error) {
	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	))
	rateLimitClient, err := github_ratelimit.NewRateLimitWaiterClient(oauthClient.Transport)
	if err != nil {
		return nil, err
	}
	// no need for uploadURL because if we upload release artifacts we will use release.UploadURL
	client, err := github.NewEnterpriseClient(baseUrl, "", rateLimitClient)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		client.UserAgent = userAgent
	}
	return &ghClient{Client: client}, nil
}

type ghClient struct {
	Client *github.Client
}

var _ GithubClient = &ghClient{}

func (g *ghClient) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]Pull, error) {
	var result []Pull
	opts := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		apiPulls, resp, err := g.Client.PullRequests.ListPullRequestsWithCommit(ctx, owner, repo, sha, opts)
		if err != nil {
			return nil, err
		}
		for _, apiPull := range apiPulls {
			if apiPull.GetMergedAt().IsZero() {
				continue
			}
			resultPull := Pull{
				Number: apiPull.GetNumber(),
				Labels: make([]string, len(apiPull.Labels)),
			}
			for i, label := range apiPull.Labels {
				resultPull.Labels[i] = label.GetName()
			}
			result = append(result, resultPull)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (g *ghClient) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error) {
	var result []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		comp, resp, err := g.Client.Repositories.CompareCommits(ctx, owner, repo, base, head, opts)
		if err != nil {
			return nil, err
		}
		for _, commit := range comp.Commits {
			result = append(result, commit.GetSHA())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (g *ghClient) GenerateReleaseNotes(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
	comp, _, err := g.Client.Repositories.GenerateReleaseNotes(ctx, owner, repo, opts)
	if err != nil {
		return "", err
	}
	return comp.Body, nil
}

func (g *ghClient) CreateRelease(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) error {
	_, _, err := g.Client.Repositories.CreateRelease(ctx, owner, repo, opts)
	return err
}

type Pull struct {
	Number      int         `json:"number"`
	Labels      []string    `json:"labels,omitempty"`
	ChangeLevel ChangeLevel `json:"change_level"`
}
