package main // must be main for plugin entry point

import (
	"context"

	lakeModels "github.com/merico-dev/lake/models"
	"github.com/merico-dev/lake/plugins/core"
	"github.com/merico-dev/lake/plugins/domainlayer/models/code"
	"github.com/merico-dev/lake/plugins/domainlayer/models/devops"
	"github.com/merico-dev/lake/plugins/domainlayer/models/ticket"
)

// plugin interface
type DomainLayer string

func (plugin DomainLayer) Init() {
	err := lakeModels.Db.AutoMigrate(
		&code.Repo{},
		&code.Commit{},
		&code.Pr{},
		&code.Note{},
		&ticket.Board{},
		&ticket.Issue{},
		&ticket.Changelog{},
		&devops.Job{},
		&devops.Build{},
	)
	if err != nil {
		panic(err)
	}
}

func (plugin DomainLayer) Description() string {
	return "Domain Layer"
}

func (plugin DomainLayer) Execute(options map[string]interface{}, progress chan<- float32, ctx context.Context) {
	progress <- 1
}

func (plugin DomainLayer) RootPkgPath() string {
	return "github.com/merico-dev/lake/plugins/domainlayer"
}

func (plugin DomainLayer) ApiResources() map[string]map[string]core.ApiResourceHandler {
	return make(map[string]map[string]core.ApiResourceHandler)
}

// Export a variable named PluginEntry for Framework to search and load
var PluginEntry DomainLayer //nolint
