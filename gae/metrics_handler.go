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

package gae

import (
	"encoding/json"
	"net/http"

	"github.com/w3c/wptdashboard/metrics"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

// This handler is responsible for pages that display aggregate metrics.
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	query := datastore.
		NewQuery("github.com.w3c.wptdashboard.metrics.MetricsRun").
		Order("-StartTime").Limit(1)
	var metadataSlice []metrics.MetricsRun

	if _, err := query.GetAll(ctx, &metadataSlice); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(metadataSlice) != 1 {
		http.Error(w, "No metrics runs found",
			http.StatusInternalServerError)
		return
	}

	metadataBytes, err := json.Marshal(metadataSlice[0])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := struct {
		Metadata string
	}{
		string(metadataBytes),
	}

	if err := templates.ExecuteTemplate(
		w, "metrics.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
