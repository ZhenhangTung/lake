package main // must be main for plugin entry point

import (
	"context"

	"github.com/merico-dev/lake/logger" // A pseudo type for Plugin Interface implementation
	lakeModels "github.com/merico-dev/lake/models"
	"github.com/merico-dev/lake/plugins/core"
	gitlabModels "github.com/merico-dev/lake/plugins/gitlab/models"
	"github.com/merico-dev/lake/plugins/gitlab/tasks"
	"github.com/merico-dev/lake/utils"
)

type Gitlab string

func (plugin Gitlab) Description() string {
	return "To collect and enrich data from Gitlab"
}

func (plugin Gitlab) Execute(options map[string]interface{}, progress chan<- float32, ctx context.Context) {
	logger.Print("start gitlab plugin execution")

	// Gilab's authenticated api rate limit is 2000 per min
	// 30 tasks/min 60s/min = 1800 per min < 2000 per min
	// You would think this would work but it hits the rate limit every time. I have to play with the number to see the right way to set it
	scheduler, err := utils.NewWorkerScheduler(50, 15, ctx)
	defer scheduler.Release()
	if err != nil {
		logger.Error("Could not create scheduler", true)
		return
	}

	projectId, ok := options["projectId"]
	if !ok {
		logger.Print("projectId is required for gitlab execution")
		return
	}

	projectIdInt := int(projectId.(float64))
	if projectIdInt < 0 {
		logger.Print("boardId is invalid")
		return
	}

	progress <- 0.1

	if err := tasks.CollectAllPipelines(projectIdInt, scheduler); err != nil {
		logger.Error("Could not collect projects: ", err)
		return
	}

	tasks.CollectChildrenOnPipelines(projectIdInt, scheduler)

	progress <- 0.2

	if err := tasks.CollectProject(projectIdInt); err != nil {
		logger.Error("Could not collect projects: ", err)
		return
	}

	progress <- 0.25

	if err := tasks.CollectCommits(projectIdInt, scheduler); err != nil {
		logger.Error("Could not collect commits: ", err)
		return
	}

	progress <- 0.3

	mergeRequestErr := tasks.CollectMergeRequests(projectIdInt, scheduler)
	if mergeRequestErr != nil {
		logger.Error("Could not collect merge requests: ", mergeRequestErr)
		return
	}

	progress <- 0.4

	collectChildrenOnMergeRequests(projectIdInt, scheduler)

	progress <- 0.8

	enrichErr := tasks.EnrichMergeRequests()
	if enrichErr != nil {
		logger.Error("Could not enrich merge requests", enrichErr)
		return
	}
	progress <- 1

	close(progress)

}

func collectNotesWithScheduler(projectIdInt int, scheduler *utils.WorkerScheduler, mrs []gitlabModels.GitlabMergeRequest) {
	for i := 0; i < len(mrs); i++ {
		mr := (mrs)[i]

		err := scheduler.Submit(func() error {
			notesErr := tasks.CollectMergeRequestNotes(projectIdInt, &mr)
			if notesErr != nil {
				logger.Error("Could not collect MR Notes", notesErr)
				return notesErr
			}

			return nil
		})
		if err != nil {
			logger.Error("err", err)
			return
		}
	}

	scheduler.WaitUntilFinish()
}
func collectCommitsWithScheduler(projectIdInt int, scheduler *utils.WorkerScheduler, mrs []gitlabModels.GitlabMergeRequest) {
	for i := 0; i < len(mrs); i++ {
		mr := (mrs)[i]

		err := scheduler.Submit(func() error {
			commitsErr := tasks.CollectMergeRequestCommits(projectIdInt, &mr)
			if commitsErr != nil {
				logger.Error("Could not collect MR Commits", commitsErr)
				return commitsErr
			}
			return nil
		})
		if err != nil {
			logger.Error("err", err)
			return
		}
	}

	scheduler.WaitUntilFinish()
}

func collectChildrenOnMergeRequests(projectIdInt int, scheduler *utils.WorkerScheduler) {
	// find all mrs from db
	var mrs []gitlabModels.GitlabMergeRequest
	lakeModels.Db.Find(&mrs)

	collectNotesWithScheduler(projectIdInt, scheduler, mrs)
	collectCommitsWithScheduler(projectIdInt, scheduler, mrs)
}

func (plugin Gitlab) RootPkgPath() string {
	return "github.com/merico-dev/lake/plugins/gitlab"
}

func (plugin Gitlab) ApiResources() map[string]map[string]core.ApiResourceHandler {
	return make(map[string]map[string]core.ApiResourceHandler)
}

// Export a variable named PluginEntry for Framework to search and load
var PluginEntry Gitlab //nolint
