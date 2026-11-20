package main

import (
	"context"
	"net/url"

	"github.com/baditaflorin/go-common/snapshotparse"
)

var savedPageHandler = snapshotparse.Handler("tos-finder", Version, parseSavedPage)

func parseSavedPage(ctx context.Context, in snapshotparse.Input) (snapshotparse.Result, error) {
	if err := ctx.Err(); err != nil {
		return snapshotparse.Result{}, err
	}
	raw, err := in.UTF8()
	if err != nil {
		return snapshotparse.Result{}, err
	}
	body := string(raw)

	page, err := url.Parse(in.URL)
	if err != nil {
		return snapshotparse.Result{}, err
	}
	findings := linkScan(body, page)
	for kind, item := range findings {
		target, err := url.Parse(item.URL)
		if err != nil || target.User != nil || !sameSiteHost(target, page) {
			delete(findings, kind)
			continue
		}
		target.RawQuery = ""
		target.ForceQuery = false
		item.URL = target.String()
		findings[kind] = item
	}
	count := len(findings)
	data := map[string]any{"document_links": findings, "documents_fetched": false}
	status := snapshotparse.Empty
	if count > 0 {
		status = snapshotparse.Parsed
	}
	return snapshotparse.Result{Status: status, Data: data}, nil
}
