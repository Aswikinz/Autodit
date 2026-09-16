package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	zen "github.com/gorules/zen-go/v2"
)

const workerArgument = "--autodit-analysis-worker"
const maxWorkerOutput = 64 * 1024 * 1024

var workers = make(chan struct{}, 2)

type RowResult struct {
	Row    int             `json:"row"`
	Input  map[string]any  `json:"input"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
	Trace  json.RawMessage `json:"trace,omitempty"`
}
type Report struct {
	Results         []RowResult `json:"results"`
	Total           int         `json:"total"`
	Flagged         int         `json:"flagged"`
	Errors          int         `json:"errors"`
	DurationMS      int64       `json:"duration_ms"`
	TracesTruncated bool        `json:"traces_truncated,omitempty"`
}
type workRequest struct {
	Model        json.RawMessage `json:"model"`
	Dataset      Dataset         `json:"dataset"`
	ValidateOnly bool            `json:"validate_only,omitempty"`
}
type workResponse struct {
	Report Report `json:"report"`
	Error  string `json:"error,omitempty"`
}

// Evaluate runs arbitrary graph code in a disposable, resource-limited process.
// Cancellation kills that process, including a blocked native or JavaScript call.
func Evaluate(ctx context.Context, model []byte, dataset Dataset) (Report, error) {
	if err := ValidateModel(model); err != nil {
		return Report{}, err
	}
	if err := ValidateDataset(dataset); err != nil {
		return Report{}, err
	}
	return runWorker(ctx, workRequest{Model: model, Dataset: dataset})
}

// ValidateSyntax parses decision expressions in the same isolated worker used
// for evaluation. The Go binding has no compile-only expression API, so parsing
// must never run with application credentials or outside the worker's limits.
func ValidateSyntax(ctx context.Context, model []byte) error {
	if err := ValidateModel(model); err != nil {
		return err
	}
	_, err := runWorker(ctx, workRequest{Model: model, ValidateOnly: true})
	return err
}

func runWorker(ctx context.Context, request workRequest) (Report, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	select {
	case workers <- struct{}{}:
		defer func() { <-workers }()
	case <-ctx.Done():
		return Report{}, ctx.Err()
	}
	data, err := json.Marshal(request)
	if err != nil {
		return Report{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return Report{}, err
	}
	cmd := exec.CommandContext(ctx, executable, workerArgument)
	// Workers have no application credentials, source passwords or configurable loader.
	cmd.Env = []string{"GOMEMLIMIT=128MiB", "GOMAXPROCS=2", "GORACE=atexit_sleep_ms=0"}
	cmd.Stdin = bytes.NewReader(data)
	output := &boundedBuffer{limit: maxWorkerOutput}
	stderr := &boundedBuffer{limit: 4096}
	cmd.Stdout = output
	cmd.Stderr = stderr
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Report{}, fmt.Errorf("analysis exceeded its time limit or was cancelled: %w", ctx.Err())
		}
		return Report{}, errors.New("analysis worker stopped; simplify the graph or reduce the dataset")
	}
	var response workResponse
	if json.Unmarshal(output.Bytes(), &response) != nil {
		return Report{}, errors.New("analysis worker returned an invalid response")
	}
	if response.Error != "" {
		return Report{}, errors.New(response.Error)
	}
	return response.Report, nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("analysis output limit exceeded")
	}
	return b.Buffer.Write(p)
}

// WorkerMain must be called before normal application or test command parsing.
// It returns false in ordinary use and true after completing a worker request.
func WorkerMain() bool {
	if len(os.Args) != 2 || os.Args[1] != workerArgument {
		return false
	}
	response := workResponse{}
	if err := limitWorker(); err != nil {
		response.Error = "analysis worker could not set resource limits"
	} else {
		var request workRequest
		dec := json.NewDecoder(io.LimitReader(os.Stdin, MaxBytes+512*1024))
		if dec.Decode(&request) != nil {
			response.Error = "invalid analysis request"
		} else if request.ValidateOnly {
			if err := ValidateModel(request.Model); err != nil {
				response.Error = err.Error()
			} else if err := validateSyntax(request.Model); err != nil {
				response.Error = err.Error()
			}
		} else {
			report, err := evaluateDirect(context.Background(), request.Model, request.Dataset)
			response.Report = report
			if err != nil {
				response.Error = err.Error()
			}
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(response)
	return true
}

func evaluateDirect(ctx context.Context, model []byte, d Dataset) (Report, error) {
	started := time.Now()
	if err := ValidateModel(model); err != nil {
		return Report{}, err
	}
	if err := ValidateDataset(d); err != nil {
		return Report{}, err
	}
	if err := validateSyntax(model); err != nil {
		return Report{}, err
	}
	engine := zen.NewEngine(zen.EngineConfig{})
	defer engine.Dispose()
	decision, err := engine.CreateDecision(model)
	if err != nil {
		return Report{}, fmt.Errorf("decision could not be compiled: %s", short(err.Error()))
	}
	defer decision.Dispose()
	report := Report{Results: make([]RowResult, 0, len(d.Rows)), Total: len(d.Rows)}
	traceBytes, outputBytes := 0, 0
	for i, row := range d.Rows {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		result := RowResult{Row: i + 1, Input: row}
		input, conversionErr := typedRow(row, d.Columns)
		if conversionErr != nil {
			result.Error = conversionErr.Error()
		} else {
			result.Input = input
			response, evalErr := decision.EvaluateWithOpts(map[string]any{"data": input}, zen.EvaluationOptions{Trace: traceBytes < 8*1024*1024, MaxDepth: 32})
			if evalErr != nil {
				result.Error = short(evalErr.Error())
			} else if len(response.Result) > 64*1024 || outputBytes+len(response.Result) > 32*1024*1024 {
				result.Error = "decision output exceeds the result size limit"
			} else {
				result.Output = response.Result
				outputBytes += len(response.Result)
				if response.Trace != nil && traceBytes+len(*response.Trace) <= 8*1024*1024 {
					result.Trace = *response.Trace
					traceBytes += len(result.Trace)
				} else {
					report.TracesTruncated = true
				}
				var value any
				if json.Unmarshal(result.Output, &value) == nil && flagged(value) {
					report.Flagged++
				}
			}
		}
		if result.Error != "" {
			report.Errors++
		}
		report.Results = append(report.Results, result)
	}
	report.DurationMS = time.Since(started).Milliseconds()
	return report, nil
}

func flagged(value any) bool {
	switch result := value.(type) {
	case map[string]any:
		return result["flag"] == true
	case []any:
		for _, item := range result {
			if flagged(item) {
				return true
			}
		}
	}
	return false
}
