package models

import (
	"github.com/merico-dev/lake/models"
)

type GitlabMergeRequestNote struct {
	GitlabId        int `gorm:"primaryKey"`
	MergeRequestId  int `gorm:"index"`
	MergeRequestIid int
	NoteableType    string
	AuthorUsername  string
	Body            string
	GitlabCreatedAt string
	Confidential    bool
	Resolvable      bool // Resolvable means a comment is a code review comment
	System          bool // System means the comment is generated automatically

	models.NoPKModel
}
