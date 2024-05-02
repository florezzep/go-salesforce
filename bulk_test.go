package salesforce

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func Test_createBulkJob(t *testing.T) {
	job := bulkJob{
		Id:    "1234",
		State: jobStateOpen,
	}
	server, sfAuth := setupTestServer(job, http.StatusOK)
	defer server.Close()

	type account struct {
		Id   string
		Name string
	}
	acc := []account{
		{
			Id:   "123",
			Name: "test account 1",
		},
		{
			Id:   "456",
			Name: "test account 2",
		},
	}
	ingestBody, _ := json.Marshal(acc)

	queryJobReq := bulkQueryJobCreationRequest{
		Operation: queryJobType,
		Query:     "SELECT Id FROM Account",
	}
	queryBody, _ := json.Marshal(queryJobReq)

	type args struct {
		auth    authentication
		jobType string
		body    []byte
	}
	tests := []struct {
		name    string
		args    args
		want    bulkJob
		wantErr bool
	}{
		{
			name: "create_bulk_ingest_job",
			args: args{
				auth:    sfAuth,
				jobType: ingestJobType,
				body:    ingestBody,
			},
			want:    job,
			wantErr: false,
		},
		{
			name: "create_bulk_query_job",
			args: args{
				auth:    sfAuth,
				jobType: queryJobType,
				body:    queryBody,
			},
			want:    job,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createBulkJob(tt.args.auth, tt.args.jobType, tt.args.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("createBulkJob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createBulkJob() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getJobResults(t *testing.T) {
	jobResults := BulkJobResults{
		Id:                  "1234",
		State:               jobStateOpen,
		NumberRecordsFailed: 0,
		ErrorMessage:        "",
	}
	server, sfAuth := setupTestServer(jobResults, http.StatusOK)
	defer server.Close()

	type args struct {
		auth      authentication
		jobType   string
		bulkJobId string
	}
	tests := []struct {
		name    string
		args    args
		want    BulkJobResults
		wantErr bool
	}{
		{
			name: "get_job_results",
			args: args{
				auth:      sfAuth,
				jobType:   ingestJobType,
				bulkJobId: "1234",
			},
			want:    jobResults,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getJobResults(tt.args.auth, tt.args.jobType, tt.args.bulkJobId)
			if (err != nil) != tt.wantErr {
				t.Errorf("getJobResults() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getJobResults() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isBulkJobDone(t *testing.T) {
	server, sfAuth := setupTestServer("example error", http.StatusOK)
	defer server.Close()

	type args struct {
		auth    authentication
		bulkJob BulkJobResults
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "bulk_job_complete",
			args: args{
				auth: authentication{},
				bulkJob: BulkJobResults{
					Id:                  "1234",
					State:               jobStateJobComplete,
					NumberRecordsFailed: 0,
					ErrorMessage:        "",
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "bulk_job_not_complete",
			args: args{
				auth: authentication{},
				bulkJob: BulkJobResults{
					Id:                  "1234",
					State:               jobStateOpen,
					NumberRecordsFailed: 0,
					ErrorMessage:        "",
				},
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "bulk_job_failed",
			args: args{
				auth: sfAuth,
				bulkJob: BulkJobResults{
					Id:                  "1234",
					State:               jobStateFailed,
					NumberRecordsFailed: 1,
					ErrorMessage:        "example error",
				},
			},
			want:    true,
			wantErr: true,
		},
		{
			name: "bulk_job_failed_records",
			args: args{
				auth: sfAuth,
				bulkJob: BulkJobResults{
					Id:                  "1234",
					State:               jobStateFailed,
					NumberRecordsFailed: 1,
					ErrorMessage:        "",
				},
			},
			want:    true,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isBulkJobDone(tt.args.auth, tt.args.bulkJob)
			if (err != nil) != tt.wantErr {
				t.Errorf("isBulkJobDone() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isBulkJobDone() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getQueryJobResults(t *testing.T) {
	csvData := `"col"` + "\n" + `"row"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Sforce-Numberofrecords", "1")
		w.Header().Add("Sforce-Locator", "")
		w.Write([]byte(csvData))
	}))
	sfAuth := authentication{
		InstanceUrl: server.URL,
		AccessToken: "accesstokenvalue",
	}
	defer server.Close()

	type args struct {
		auth      authentication
		bulkJobId string
		locator   string
	}
	tests := []struct {
		name    string
		args    args
		want    bulkJobQueryResults
		wantErr bool
	}{
		{
			name: "get_single_query_job_result",
			args: args{
				auth:      sfAuth,
				bulkJobId: "1234",
				locator:   "",
			},
			want: bulkJobQueryResults{
				NumberOfRecords: 1,
				Locator:         "",
				Data:            [][]string{{"col"}, {"row"}},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getQueryJobResults(tt.args.auth, tt.args.bulkJobId, tt.args.locator)
			if (err != nil) != tt.wantErr {
				t.Errorf("getQueryJobResults() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getQueryJobResults() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_mapsToCSV(t *testing.T) {
	type args struct {
		maps []map[string]any
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "convert_map_to_csv_string",
			args: args{
				maps: []map[string]any{
					{
						"key": "val",
					},
					{
						"key": "val1",
					},
				},
			},
			want:    "key\nval\nval1\n",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapsToCSV(tt.args.maps)
			if (err != nil) != tt.wantErr {
				t.Errorf("mapsToCSV() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("mapsToCSV() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_mapsToCSVSlices(t *testing.T) {
	type args struct {
		maps []map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    [][]string
		wantErr bool
	}{
		{
			name: "convert_map_to_slice_of_strings",
			args: args{
				maps: []map[string]string{
					{
						"key": "val",
					},
					{
						"key": "val1",
					},
				},
			},
			want:    [][]string{{"key"}, {"val"}, {"val1"}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapsToCSVSlices(tt.args.maps)
			if (err != nil) != tt.wantErr {
				t.Errorf("mapsToCSVSlices() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mapsToCSVSlices() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_constructBulkJobRequest(t *testing.T) {
	job := bulkJob{
		Id:    "1234",
		State: jobStateOpen,
	}
	server, sfAuth := setupTestServer(job, http.StatusOK)
	defer server.Close()

	badJob := bulkJob{
		Id:    "1234",
		State: jobStateAborted,
	}
	badJobByte, _ := json.Marshal(badJob)
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(badJobByte)
	}))
	badSfAuth := authentication{
		InstanceUrl: badServer.URL,
		AccessToken: "accesstokenvalue",
	}
	defer badServer.Close()

	type args struct {
		auth        authentication
		sObjectName string
		operation   string
		fieldName   string
	}
	tests := []struct {
		name    string
		args    args
		want    bulkJob
		wantErr bool
	}{
		{
			name: "construct_bulk_job_success",
			args: args{
				auth:        sfAuth,
				sObjectName: "Account",
				operation:   insertOperation,
				fieldName:   "",
			},
			want:    job,
			wantErr: false,
		},
		{
			name: "construct_bulk_job_fail",
			args: args{
				auth:        badSfAuth,
				sObjectName: "Account",
				operation:   insertOperation,
				fieldName:   "",
			},
			want:    badJob,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := constructBulkJobRequest(tt.args.auth, tt.args.sObjectName, tt.args.operation, tt.args.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("constructBulkJobRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("constructBulkJobRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_doBulkJob(t *testing.T) {
	type account struct {
		ExternalId string
		Name       string
	}
	job := bulkJob{
		Id:    "1234",
		State: jobStateOpen,
	}
	server, sfAuth := setupTestServer(job, http.StatusOK)
	defer server.Close()

	type args struct {
		auth           authentication
		sObjectName    string
		fieldName      string
		operation      string
		records        any
		batchSize      int
		waitForResults bool
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "bulk_insert",
			args: args{
				auth:        sfAuth,
				sObjectName: "Account",
				fieldName:   "",
				operation:   insertOperation,
				records: []account{
					{
						Name: "test account 1",
					},
					{
						Name: "test account 2",
					},
				},
				batchSize:      200,
				waitForResults: false,
			},
			want:    []string{job.Id},
			wantErr: false,
		},
		{
			name: "bulk_upsert",
			args: args{
				auth:        sfAuth,
				sObjectName: "Account",
				fieldName:   "externalId",
				operation:   upsertOperation,
				records: []account{
					{
						ExternalId: "acc1",
						Name:       "test account 1",
					},
					{
						ExternalId: "acc2",
						Name:       "test account 2",
					},
				},
				batchSize:      200,
				waitForResults: false,
			},
			want:    []string{job.Id},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := doBulkJob(tt.args.auth, tt.args.sObjectName, tt.args.fieldName, tt.args.operation, tt.args.records, tt.args.batchSize, tt.args.waitForResults)
			if (err != nil) != tt.wantErr {
				t.Errorf("doBulkJob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("doBulkJob() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getFailedRecords(t *testing.T) {
	server, sfAuth := setupTestServer("error", http.StatusOK)
	defer server.Close()

	type args struct {
		auth      authentication
		bulkJobId string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "get_failed_records",
			args: args{
				auth:      sfAuth,
				bulkJobId: "1234",
			},
			want:    "\"error\"",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getFailedRecords(tt.args.auth, tt.args.bulkJobId)
			if (err != nil) != tt.wantErr {
				t.Errorf("getFailedRecords() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getFailedRecords() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_waitForJobResult(t *testing.T) {
	jobResults := BulkJobResults{
		Id:    "1234",
		State: jobStateJobComplete,
	}
	server, sfAuth := setupTestServer(jobResults, http.StatusOK)
	defer server.Close()

	type args struct {
		auth      authentication
		bulkJobId string
		jobType   string
		interval  time.Duration
		c         chan error
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "wait_for_ingest_result",
			args: args{
				auth:      sfAuth,
				bulkJobId: "1234",
				jobType:   ingestJobType,
				interval:  time.Nanosecond,
				c:         make(chan error),
			},
			wantErr: false,
		},
		{
			name: "wait_for_query_result",
			args: args{
				auth:      sfAuth,
				bulkJobId: "1234",
				jobType:   queryJobType,
				interval:  time.Nanosecond,
				c:         make(chan error),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			go waitForJobResultsAsync(tt.args.auth, tt.args.bulkJobId, tt.args.jobType, tt.args.interval, tt.args.c)
			err := <-tt.args.c
			if (err != nil) != tt.wantErr {
				t.Errorf("waitForJobResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_waitForQueryResults(t *testing.T) {
	jobResults := BulkJobResults{
		Id:    "1234",
		State: jobStateJobComplete,
	}
	server, sfAuth := setupTestServer(jobResults, http.StatusOK)
	defer server.Close()

	type args struct {
		auth      authentication
		bulkJobId string
		jobType   string
		interval  time.Duration
	}
	tests := []struct {
		name    string
		args    args
		want    [][]string
		wantErr bool
	}{
		{
			name: "wait_for_ingest_result",
			args: args{
				auth:      sfAuth,
				bulkJobId: "1234",
				jobType:   ingestJobType,
				interval:  time.Nanosecond,
			},
			wantErr: false,
		},
		{
			name: "wait_for_query_result",
			args: args{
				auth:      sfAuth,
				bulkJobId: "1234",
				jobType:   queryJobType,
				interval:  time.Nanosecond,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := waitForJobResults(tt.args.auth, tt.args.bulkJobId, tt.args.jobType, tt.args.interval)
			if (err != nil) != tt.wantErr {
				t.Errorf("waitForQueryResults() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_collectQueryResults(t *testing.T) {
	csvData := `"col"` + "\n" + `"row"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "?locator=") {
			w.Header().Add("Sforce-Locator", "")
		} else {
			w.Header().Add("Sforce-Locator", "abc")
		}
		w.Header().Add("Sforce-Numberofrecords", "1")
		w.Write([]byte(csvData))
	}))
	sfAuth := authentication{
		InstanceUrl: server.URL,
		AccessToken: "accesstokenvalue",
	}
	defer server.Close()

	type args struct {
		auth      authentication
		bulkJobId string
	}
	tests := []struct {
		name    string
		args    args
		want    [][]string
		wantErr bool
	}{
		{
			name: "query_with_locator",
			args: args{
				auth:      sfAuth,
				bulkJobId: "123",
			},
			want:    [][]string{{"col"}, {"row"}, {"row"}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := collectQueryResults(tt.args.auth, tt.args.bulkJobId)
			if (err != nil) != tt.wantErr {
				t.Errorf("collectQueryResults() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("collectQueryResults() = %v, want %v", got, tt.want)
			}
		})
	}
}
