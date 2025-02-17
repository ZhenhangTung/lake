package tasks

import (
	"fmt"
	"net/http"

	"github.com/merico-dev/lake/logger"
	lakeModels "github.com/merico-dev/lake/models"
	"github.com/merico-dev/lake/plugins/core"
	"github.com/merico-dev/lake/plugins/github/models"
	"github.com/merico-dev/lake/utils"
	"gorm.io/gorm/clause"
)

type ApiIssuesResponse []IssuesResponse
type IssuesResponse struct {
	GithubId    int `json:"id"`
	Number      int
	State       string
	Title       string
	Body        string
	PullRequest struct {
		Url     string `json:"url"`
		HtmlUrl string `json:"html_url"`
	} `json:"pull_request"`
	ClosedAt        string `json:"closed_at"`
	GithubCreatedAt string `json:"created_at"`
	GithubUpdatedAt string `json:"updated_at"`
}

type Pull struct {
	GithubId        int `json:"id"`
	State           string
	Title           string
	Number          int
	CommentsUrl     string `json:"comments_url"`
	CommitsUrl      string `json:"commits_url"`
	HTMLUrl         string `json:"html_url"`
	MergedAt        string `json:"merged_at"`
	GithubCreatedAt string `json:"created_at"`
	ClosedAt        string `json:"closed_at"`
}

func CollectIssues(owner string, repositoryName string, repositoryId int, scheduler *utils.WorkerScheduler) error {
	githubApiClient := CreateApiClient()
	getUrl := fmt.Sprintf("repos/%v/%v/issues?state=all", owner, repositoryName)
	return githubApiClient.FetchWithPaginationAnts(getUrl, 100, 20, scheduler,
		func(res *http.Response) error {
			githubApiResponse := &ApiIssuesResponse{}
			err := core.UnmarshalResponse(res, githubApiResponse)
			if err != nil {
				logger.Error("Error: ", err)
				return err
			}

			for _, issue := range *githubApiResponse {
				if issue.PullRequest.Url == "" {
					// This is an issue from github
					githubIssue := &models.GithubIssue{
						GithubId:        issue.GithubId,
						Number:          issue.Number,
						State:           issue.State,
						Title:           issue.Title,
						Body:            issue.Body,
						ClosedAt:        utils.ConvertStringToTime(issue.ClosedAt),
						GithubCreatedAt: utils.ConvertStringToTime(issue.GithubCreatedAt),
						GithubUpdatedAt: utils.ConvertStringToTime(issue.GithubUpdatedAt),
					}

					err = lakeModels.Db.Clauses(clause.OnConflict{
						UpdateAll: true,
					}).Create(&githubIssue).Error
					if err != nil {
						logger.Error("Could not upsert: ", err)
					}
				} else {
					// This is a pull request from github
					githubPull := &models.GithubPullRequest{
						GithubId:        issue.GithubId,
						RepositoryId:    repositoryId,
						Number:          issue.Number,
						State:           issue.State,
						Title:           issue.Title,
						GithubCreatedAt: issue.GithubCreatedAt,
						ClosedAt:        issue.ClosedAt,
					}
					err = lakeModels.Db.Debug().Clauses(clause.OnConflict{
						UpdateAll: true,
					}).Create(&githubPull).Error
					if err != nil {
						logger.Error("Could not upsert: ", err)
					}
				}
			}
			return nil
		})
}
