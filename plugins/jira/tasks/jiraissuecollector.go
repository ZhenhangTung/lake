package tasks

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/merico-dev/lake/utils"

	lakeModels "github.com/merico-dev/lake/models"
	"github.com/merico-dev/lake/plugins/core"
	"github.com/merico-dev/lake/plugins/jira/models"
	"gorm.io/gorm/clause"
)

type JiraApiIssue struct {
	Id     string                 `json:"id"`
	Self   string                 `json:"self"`
	Key    string                 `json:"key"`
	Fields map[string]interface{} `json:"fields"`
}

type JiraApiIssuesResponse struct {
	JiraPagination
	Issues []JiraApiIssue `json:"issues"`
}

func CollectIssues(jiraApiClient *JiraApiClient, source *models.JiraSource, boardId uint64, since time.Time, ctx context.Context) error {
	// diff sync
	var latestUpdated models.JiraIssue
	err := lakeModels.Db.Order("updated DESC").Limit(1).Find(&latestUpdated).Error
	if err != nil {
		return err
	}
	jql := "ORDER BY updated ASC"

	if latestUpdated.IssueId > 0 {
		// This is not the first time we have fetched data for Jira.
		jql = fmt.Sprintf("updated >= '%v' %v", latestUpdated.Updated.Format("2006/01/02 15:04"), jql)
	} else if !since.IsZero() {
		fmt.Println("KEVIN >>> time is not zero")
		// This is the first time we are fetching data from Jira and the user has sent an update time.
		// We don't want all the data, we only want data since the update time.
		jql = fmt.Sprintf("updated >= '%v' %v", since.Format("2006/01/02 15:04"), jql)
	}
	// Otherwise, we fetch all the data from all time
	query := &url.Values{}
	query.Set("jql", jql)

	scheduler, err := utils.NewWorkerScheduler(10, 50, ctx)
	if err != nil {
		return err
	}
	defer scheduler.Release()

	err = jiraApiClient.FetchPages(scheduler, fmt.Sprintf("/agile/1.0/board/%v/issue", boardId), query,
		func(res *http.Response) error {
			// parse response
			jiraApiIssuesResponse := &JiraApiIssuesResponse{}
			err := core.UnmarshalResponse(res, jiraApiIssuesResponse)
			if err != nil {
				return err
			}

			// process issues
			for _, jiraApiIssue := range jiraApiIssuesResponse.Issues {

				jiraIssue, err := convertIssue(source, &jiraApiIssue)
				if err != nil {
					return err
				}
				// issue
				err = lakeModels.Db.Clauses(clause.OnConflict{
					UpdateAll: true,
				}).Create(jiraIssue).Error
				if err != nil {
					return err
				}

				// board / issue relationship
				lakeModels.Db.Create(&models.JiraBoardIssue{
					SourceId: source.ID,
					BoardId:  boardId,
					IssueId:  jiraIssue.IssueId,
				})
			}
			return nil
		})
	if err != nil {
		return err
	}
	scheduler.WaitUntilFinish()
	return nil
}

func convertIssue(source *models.JiraSource, jiraApiIssue *JiraApiIssue) (*models.JiraIssue, error) {
	id, err := strconv.ParseUint(jiraApiIssue.Id, 10, 64)
	if err != nil {
		return nil, err
	}
	created, err := time.Parse(core.ISO_8601_FORMAT, jiraApiIssue.Fields["created"].(string))
	if err != nil {
		return nil, err
	}
	updated, err := time.Parse(core.ISO_8601_FORMAT, jiraApiIssue.Fields["updated"].(string))
	if err != nil {
		return nil, err
	}
	projectId, err := strconv.ParseUint(
		jiraApiIssue.Fields["project"].(map[string]interface{})["id"].(string), 10, 64,
	)
	if err != nil {
		return nil, err
	}
	status := jiraApiIssue.Fields["status"].(map[string]interface{})
	statusName := status["name"].(string)
	statusKey := status["statusCategory"].(map[string]interface{})["key"].(string)
	statusCategory := status["statusCategory"].(map[string]interface{})["name"].(string)
	epicKey := ""
	if source.EpicKeyField != "" {
		epicKey, _ = jiraApiIssue.Fields[source.EpicKeyField].(string)
	}
	resolutionDate := sql.NullTime{}
	if rd, ok := jiraApiIssue.Fields["resolutiondate"]; ok && rd != nil {
		if resolutionDate.Time, err = time.Parse(core.ISO_8601_FORMAT, rd.(string)); err == nil {
			resolutionDate.Valid = true
		}
	}
	workload := 0.0
	if source.StoryPointField != "" {
		workload, _ = jiraApiIssue.Fields[source.StoryPointField].(float64)
	}
	jiraIssue := &models.JiraIssue{
		SourceId:       source.ID,
		IssueId:        id,
		ProjectId:      projectId,
		Self:           jiraApiIssue.Self,
		Key:            jiraApiIssue.Key,
		Summary:        jiraApiIssue.Fields["summary"].(string),
		Type:           jiraApiIssue.Fields["issuetype"].(map[string]interface{})["name"].(string),
		StatusName:     statusName,
		StatusKey:      statusKey,
		StatusCategory: statusCategory,
		EpicKey:        epicKey,
		ResolutionDate: resolutionDate,
		StoryPoint:     workload,
		Created:        created,
		Updated:        updated,
	}
	return jiraIssue, nil
}
