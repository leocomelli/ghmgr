package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	gh "github.com/google/go-github/github"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
	yaml "gopkg.in/yaml.v2"
)

const (
	fileName      = "config.yml"
	commitMessage = "updated %s"
)

type Configuration struct {
	Source struct {
		URL          string
		Token        string
		Organization string
		Instance     *gh.Client
		Ignore       []string
		Archive      bool
		Content      struct {
			Path    string
			Message string
		}
	}
	Target struct {
		URL          string
		Token        string
		Organization string
		Instance     *gh.Client
	}
	Git struct {
		ClonePath  string `yaml:"clone_path"`
		RemoteName string `yaml:"remote_name"`
		CrtFile    string `yaml:"ctr_file"`
		Author     string `yaml:"commit_author"`
		Email      string `yaml:"commit_email"`
	}
}

func newGithubClient(token, URL string) *gh.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
	ctx := context.WithValue(oauth2.NoContext, oauth2.HTTPClient, client)
	tc := oauth2.NewClient(ctx, ts)

	if URL == "" {
		return gh.NewClient(tc)
	}
	c, err := gh.NewEnterpriseClient(URL, URL, tc)
	if err != nil {
		log.Fatal(err)
	}
	return c
}

func loadConfiguration(configPath string) (*Configuration, error) {
	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	c := &Configuration{}
	yaml.Unmarshal(content, c)

	return c, nil
}

func main() {
	cfg, err := loadConfiguration(fileName)
	if err != nil {
		log.Fatal(err)
	}

	cfg.Source.Instance = newGithubClient(cfg.Source.Token, cfg.Source.URL)
	cfg.Target.Instance = newGithubClient(cfg.Target.Token, cfg.Target.URL)

	log.WithField("url", cfg.Source.URL).Warn("source github")
	log.WithField("url", cfg.Target.URL).Warn("target github")

	repos, err := listRepositoriesByOrg(cfg)
	if err != nil {
		log.Fatal(err)
	}

	log.WithField("amount", len(repos)).Info("some repositories was found")
	log.WithField("names", cfg.Source.Ignore).Info("ignoring some repositories")

	for i, repo := range repos {
		log.WithField("name", *repo.Name).WithField("index", fmt.Sprintf("%d/%d", i+1, len(repos))).
			Info("processing a repository")

		r, err := createRepo(cfg, repo)
		if err != nil {
			log.Error(err)
			continue
		}

		err = cloneAndPush(cfg, repo, *r.SSHURL)
		if err != nil {
			log.Error(err)
			continue
		}

		if cfg.Source.Content.Path != "" {
			err := updateContent(cfg, r)
			if err != nil {
				log.Error(err)
			}
		}

		if cfg.Source.Archive {
			archiveRepo(cfg, repo)
			if err != nil {
				log.Error(err)
			}
		}
		log.Info("done =-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-")
		break
	}
}

func contains(sl []string, v string) bool {
	for _, vv := range sl {
		if vv == v {
			return true
		}
	}
	return false
}

func listRepositoriesByOrg(cfg *Configuration) ([]*gh.Repository, error) {
	source := cfg.Source
	opts := &gh.RepositoryListByOrgOptions{
		ListOptions: gh.ListOptions{PerPage: 30},
	}

	var candidates []*gh.Repository
	for {
		repos, resp, err := source.Instance.Repositories.List(context.Background(), source.Organization, &gh.RepositoryListOptions{})
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, repos...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	var allRepos []*gh.Repository
	for _, r := range candidates {
		if !contains(cfg.Source.Ignore, *r.Name) {
			allRepos = append(allRepos, r)
		}
	}

	return allRepos, nil
}

func createRepo(cfg *Configuration, repo *gh.Repository) (*gh.Repository, error) {
	ctx := context.Background()

	opts := &gh.Repository{
		Name:             repo.Name,
		Description:      repo.Description,
		Homepage:         repo.Homepage,
		Private:          repo.Private,
		HasIssues:        gh.Bool(false),
		HasProjects:      gh.Bool(false),
		HasWiki:          gh.Bool(false),
		AllowRebaseMerge: gh.Bool(false),
		AllowSquashMerge: gh.Bool(false),
	}

	r, _, err := cfg.Target.Instance.Repositories.Create(ctx, cfg.Target.Organization, opts)
	if err != nil {
		return nil, err
	}

	log.WithField("url", *r.URL).Info("a new repository was created successfully")

	return r, nil
}

func cloneAndPush(cfg *Configuration, source *gh.Repository, targetURL string) error {

	log.WithField("file", cfg.Git.CrtFile).Info("using the public key...")
	auth, err := ssh.NewPublicKeysFromFile("git", cfg.Git.CrtFile, "")
	if err != nil {
		return err
	}

	log.WithField("url", *source.SSHURL).Info("cloning the repository...")

	g, err := git.PlainClone(fmt.Sprintf("%s/%s", cfg.Git.ClonePath, *source.Name), true, &git.CloneOptions{
		URL:  *source.SSHURL,
		Auth: auth,
	})

	if err != nil {
		return err
	}

	log.WithField("remote", targetURL).Info("adding a new remote...")

	_, err = g.CreateRemote(&config.RemoteConfig{
		Name: cfg.Git.RemoteName,
		URLs: []string{targetURL},
	})
	if err != nil {
		return err
	}

	log.WithField("remote", targetURL).Info("pushing to the new remote...")

	err = g.Push(&git.PushOptions{
		RemoteName: cfg.Git.RemoteName,
		Auth:       auth,
	})
	if err != nil {
		return err
	}

	return nil
}

func updateContent(cfg *Configuration, repo *gh.Repository) error {
	ctx := context.Background()
	source := cfg.Source

	c, _, _, err := source.Instance.Repositories.GetContents(ctx, source.Organization, *repo.Name, source.Content.Path, &gh.RepositoryContentGetOptions{})
	if err != nil {
		return err
	}

	content, err := c.GetContent()
	if err != nil {
		return err
	}

	log.WithField("filename", source.Content.Path).Info("updating the content...")

	newMessage := strings.Replace(source.Content.Message, "{{url}}", *repo.HTMLURL, -1)

	repositoryContentsOptions := &gh.RepositoryContentFileOptions{
		Message:   gh.String(fmt.Sprintf(commitMessage, source.Content.Path)),
		Content:   []byte(fmt.Sprintf("%s<br><br>%s", newMessage, content)),
		SHA:       gh.String(c.GetSHA()),
		Committer: &gh.CommitAuthor{Name: gh.String(cfg.Git.Author), Email: gh.String(cfg.Git.Email)},
	}

	_, _, err = source.Instance.Repositories.UpdateFile(ctx, source.Organization, *repo.Name, source.Content.Path, repositoryContentsOptions)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}

func archiveRepo(cfg *Configuration, repo *gh.Repository) error {
	ctx := context.Background()
	source := cfg.Source

	opts := &gh.Repository{
		Archived: gh.Bool(true),
	}

	log.WithField("name", *repo.Name).Info("archiving the repository...")

	_, _, err := source.Instance.Repositories.Edit(ctx, source.Organization, *repo.Name, opts)
	if err != nil {
		return err
	}

	return nil
}
