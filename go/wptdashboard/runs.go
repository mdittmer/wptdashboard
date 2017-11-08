// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wptdashboard

import (
	"cloud.google.com/go/datastore"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
)

func TestRunsForShaAndBrowsers(ctx context.Context, client *datastore.Client, runSHA string, browserNames []string) (testRuns []TestRun, err error) {
	baseQuery := datastore.NewQuery("TestRun").Order("-CreatedAt").Limit(1)

	for _, browserName := range browserNames {
		query := baseQuery.Filter("BrowserName =", browserName)
		if runSHA != "" && runSHA != "latest" {
			query = query.Filter("Revision =", runSHA)
		}
		for i := client.Run(ctx, query); ; {
			var testRun TestRun
			_, err := i.Next(&testRun)
			if err == iterator.Done {
				break
			}
			if err != nil {
				return testRuns, err
			}
			testRuns = append(testRuns, testRun)
		}
	}

	return testRuns, nil
}

func TestRunsForShaAndBrowser(ctx context.Context, client *datastore.Client, runSHA string, browserName string) (testRun *TestRun, err error) {
	query := datastore.
		NewQuery("TestRun").
		Order("-CreatedAt").
		Limit(1).
		Filter("BrowserName =", browserName)
	if runSHA != "" && runSHA != "latest" {
		query = query.Filter("Revision =", runSHA)
	}

	for i := client.Run(ctx, query); ; {
		var testRun TestRun
		_, err := i.Next(&testRun)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return &testRun, err
		}
		return &testRun, nil
	}

	return nil, nil
}
