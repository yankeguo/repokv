package main

import "context"

type UpdateRepoKeyValueParams struct {
	Dir        string
	Repo       RepoConf
	Data       map[string]string
	MaxRetries int
}

func UpdateRepoKeyValue(ctx context.Context, params UpdateRepoKeyValueParams) (err error) {
	return
}
