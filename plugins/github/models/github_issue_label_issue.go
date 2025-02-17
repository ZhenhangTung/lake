package models

import (
	"github.com/merico-dev/lake/models"
)

// This Model is intended to be an association table between issue labels and issues.
// It needs to exist because there is a many to many relationship between issue labels
// (which are labels associated to a issue) and issues.

// Also note that Pull Requests are considered to be Issues in GitHub. This means that
// an Issue Id can be considered a Pull Request Id also.

type GithubIssueLabelIssue struct {
	IssueLabelId int `gorm:"index"`
	IssueId      int `gorm:"index"`
	models.NoPKModel
}
