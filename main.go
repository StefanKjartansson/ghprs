package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.google.com/p/goauth2/oauth"
	"github.com/google/go-github/github"
	"github.com/hashicorp/hcl"
	"github.com/mitchellh/colorstring"
	"github.com/mitchellh/go-homedir"
)

var (
	staleDate = time.Now().Truncate(time.Duration(time.Hour * 24 * 30))
)

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func rightPad2Len(s string, padStr string, overallLen int) string {
	var padCountInt int
	padCountInt = 1 + ((overallLen - len(padStr)) / len(padStr))
	var retStr = s + strings.Repeat(padStr, padCountInt)
	return retStr[:overallLen]
}

func getOrgRepos(client *github.Client, org string) (chan github.Repository, chan error) {

	repoChan := make(chan github.Repository)
	errChan := make(chan error)

	go func() {

		nextPage := true
		options := github.RepositoryListByOrgOptions{ListOptions: github.ListOptions{Page: 1}}

		for nextPage {
			repos, response, err := client.Repositories.ListByOrg(org, &options)
			if err != nil {
				errChan <- err
			}
			for _, r := range repos {
				repoChan <- r
			}
			if response.NextPage == 0 {
				nextPage = false
				close(repoChan)
				close(errChan)
			} else {
				options.Page++
			}
		}

	}()

	return repoChan, errChan

}

func getPullRequests(client *github.Client, org, repo string) (chan *github.PullRequest, chan error) {

	prChan := make(chan *github.PullRequest)
	errChan := make(chan error)

	go func() {

		nextPage := true
		options := github.PullRequestListOptions{ListOptions: github.ListOptions{Page: 1}}

		for nextPage {

			prs, response, err := client.PullRequests.List(org, repo, &options)
			if err != nil {
				errChan <- err
			}

			var wg sync.WaitGroup

			for _, p := range prs {
				wg.Add(1)
				go func(number int) {
					defer wg.Done()
					pr, _, err := client.PullRequests.Get(org, repo, number)
					if err != nil {
						errChan <- err
					} else {
						prChan <- pr
					}
				}(*p.Number)
			}
			wg.Wait()

			if response.NextPage == 0 {
				nextPage = false
				close(prChan)
				close(errChan)
			} else {
				options.Page++
			}
		}

	}()

	return prChan, errChan

}

type PullRequestLister struct {
	org    string
	client *github.Client
}

func NewPullRequestLister(org, token string) *PullRequestLister {

	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: token},
	}

	return &PullRequestLister{
		org:    org,
		client: github.NewClient(t.Client()),
	}

}

func (m *PullRequestLister) Run(whitelist ...string) (err error) {

	repoChan, repoErrChan := getOrgRepos(m.client, m.org)

	for repoChan != nil {

		select {

		case repo, ok := <-repoChan:

			if !ok {
				repoChan = nil
				continue
			}

			if len(whitelist) > 0 && !stringInSlice(*repo.Name, whitelist) {
				continue
			}

			language := "Unknown"
			if repo.Language != nil {
				language = *repo.Language
			}

			fmt.Println(colorstring.Color(fmt.Sprintf("[bold][light_green]%s[reset] [blue][%s]", *repo.Name, language)))

			prChan, prErrChan := getPullRequests(m.client, m.org, *repo.Name)

			for prChan != nil {

				select {

				case pr, ok := <-prChan:

					if !ok {
						prChan = nil
						continue
					}

					prstr := fmt.Sprintf("  [bold][white]#%s[reset][cyan]%q ", rightPad2Len(strconv.Itoa(*pr.Number), " ", 4), *pr.Title)

					mergeable := false
					if pr.Mergeable != nil {
						mergeable = *pr.Mergeable
					}

					if mergeable == true {
						prstr += "[green]mergable"
					} else {
						prstr += "[red]unmergable"
					}

					if pr.UpdatedAt.Before(staleDate) {
						prstr += " [reset][bold][underline][yellow]very-old"
					}

					fmt.Println(colorstring.Color(prstr))

				case err = <-prErrChan:
					if err != nil {
						return
					}
				}
			}

		case err = <-repoErrChan:
			if err != nil {
				return
			}

		}

	}

	return
}

func loadVarFile(path string) (map[string]string, error) {
	// Read the HCL file and prepare for parsing
	d, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf(
			"Error reading %s: %s", path, err)
	}

	// Parse it
	obj, err := hcl.Parse(string(d))
	if err != nil {
		return nil, fmt.Errorf(
			"Error parsing %s: %s", path, err)
	}

	var result map[string]string
	if err := hcl.DecodeObject(&result, obj); err != nil {
		return nil, err
	}

	return result, nil
}

func main() {

	fullPath, err := homedir.Expand("~/.ghprs")
	if err != nil {
		log.Fatalf("Failed to expand home directory: %v", err)
	}

	vars, err := loadVarFile(fullPath)
	if err != nil {
		log.Fatalf("Failed to load configuration file: %v", err)
	}

	fmt.Println(colorstring.Color("[green]Connecting to GitHub..."))

	organization, ok := vars["organization"]

	if !ok {
		log.Fatalf("Configuration file is missing organization")
	}

	token, ok := vars["token"]

	if !ok {
		log.Fatalf("Configuration file is missing token")
	}

	m := NewPullRequestLister(organization, token)

	err = m.Run(os.Args[1:]...)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}
