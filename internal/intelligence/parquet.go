package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	parquet "github.com/parquet-go/parquet-go"
)

const maxDatasetBytes int64 = 64 << 20

func (h *huggingFace) useParquet() bool {
	if h.cfg.Transport == "rows" {
		return false
	}
	if h.cfg.Transport == "parquet" {
		return true
	}
	return (h.mode == "livebench" || h.mode == "lmarena") && h.cfg.URL == "https://datasets-server.huggingface.co/rows"
}

type datasetManifest struct {
	SHA      string `json:"sha"`
	Siblings []struct {
		Filename string `json:"rfilename"`
	} `json:"siblings"`
	Card struct {
		Configs []struct {
			Name  string `json:"config_name"`
			Files []struct {
				Split string `json:"split"`
				Path  any    `json:"path"`
			} `json:"data_files"`
		} `json:"configs"`
	} `json:"cardData"`
}

func safeDatasetPath(value string) bool {
	return value != "" && !strings.ContainsAny(value, "\\?#\x00") && !strings.HasPrefix(value, "/") && path.Clean(value) == value && value != ".." && !strings.HasPrefix(value, "../")
}

func (h *huggingFace) discoverFiles(ctx context.Context, dataset Dataset) (string, []string, error) {
	if !safeDatasetPath(dataset.ID) {
		return "", nil, &SourceError{Stage: "dataset identity"}
	}
	data, err := h.get(ctx, "https://huggingface.co/api/datasets/"+dataset.ID+"/revision/main", "Authorization")
	if err != nil {
		return "", nil, err
	}
	var manifest datasetManifest
	if json.Unmarshal(data, &manifest) != nil || len(manifest.SHA) != 40 || strings.Trim(manifest.SHA, "0123456789abcdef") != "" {
		return "", nil, &SourceError{Stage: "dataset revision manifest"}
	}
	var patterns []string
	for _, config := range manifest.Card.Configs {
		if config.Name != dataset.Config {
			continue
		}
		for _, file := range config.Files {
			if file.Split != dataset.Split {
				continue
			}
			switch value := file.Path.(type) {
			case string:
				patterns = append(patterns, value)
			case []any:
				for _, entry := range value {
					if p, ok := entry.(string); ok {
						patterns = append(patterns, p)
					}
				}
			}
		}
	}
	// Hub's default native-Parquet layout can omit an explicit configs block.
	if len(manifest.Card.Configs) == 0 && dataset.Config == "default" && safeDatasetPath(dataset.Split) && !strings.ContainsAny(dataset.Split, "/*[]") {
		patterns = []string{"data/" + dataset.Split + "-*.parquet"}
	}
	files := map[string]bool{}
	for _, sibling := range manifest.Siblings {
		if !safeDatasetPath(sibling.Filename) || !strings.HasSuffix(sibling.Filename, ".parquet") {
			continue
		}
		for _, pattern := range patterns {
			matched, err := path.Match(pattern, sibling.Filename)
			if err != nil {
				return "", nil, &SourceError{Stage: "dataset file pattern"}
			}
			if matched {
				files[sibling.Filename] = true
			}
		}
	}
	if len(files) == 0 || len(files) > 128 {
		return "", nil, &SourceError{Stage: "native Parquet split discovery"}
	}
	return manifest.SHA, tableKeys(files), nil
}

func benchmarkDownloadRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 5 || req.URL.Scheme != "https" || req.URL.User != nil || (req.URL.Port() != "" && req.URL.Port() != "443") {
		return errors.New("unsafe benchmark download redirect")
	}
	host := strings.ToLower(req.URL.Hostname())
	if host != "huggingface.co" && host != "cdn-lfs.huggingface.co" && !strings.HasSuffix(host, ".cdn.hf.co") && host != "cas-bridge.xethub.hf.co" && host != "cdn-lfs-us-1.huggingface.co" && host != "cdn-lfs-eu-1.huggingface.co" {
		return errors.New("untrusted benchmark download host")
	}
	if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		req.Header.Del("Authorization")
	}
	return nil
}

func (h *huggingFace) fetchParquet(ctx context.Context) (Result, error) {
	result := Result{Metadata: map[string]any{"datasets": h.cfg.Datasets, "transport": "Hugging Face native Parquet files"}}
	revisions := map[string]string{}
	remainingBytes, remainingRows := maxDatasetBytes, int64(h.cfg.MaxRows)
	client := *h.client
	client.CheckRedirect = benchmarkDownloadRedirect
	for _, dataset := range h.cfg.Datasets {
		reportActivity(ctx, "discovering", "Discovering published dataset files")
		revision, files, err := h.discoverFiles(ctx, dataset)
		if err != nil {
			return Result{}, err
		}
		if previous := revisions[dataset.ID]; previous != "" && previous != revision {
			return Result{}, &SourceError{Stage: "dataset changed during discovery"}
		}
		revisions[dataset.ID] = revision
		for index, filename := range files {
			reportActivity(ctx, "downloading", fmt.Sprintf("Downloading dataset file %d of %d", index+1, len(files)))
			observations, bytes, rows, err := h.fetchParquetFile(ctx, &client, dataset, revision, filename, remainingBytes, remainingRows)
			if err != nil {
				return Result{}, err
			}
			remainingBytes -= bytes
			remainingRows -= rows
			result.Observations = append(result.Observations, observations...)
		}
	}
	result.Metadata["revisions"] = revisions
	return result, nil
}

func (h *huggingFace) fetchParquetFile(ctx context.Context, client *http.Client, dataset Dataset, revision, filename string, byteLimit, rowLimit int64) ([]Observation, int64, int64, error) {
	file, err := os.CreateTemp("", "torii-benchmark-*.parquet")
	if err != nil {
		return nil, 0, 0, &SourceError{Stage: "temporary dataset storage"}
	}
	defer os.Remove(file.Name())
	defer file.Close()
	u := &url.URL{Scheme: "https", Host: "huggingface.co", Path: "/datasets/" + dataset.ID + "/resolve/" + revision + "/" + filename}
	var size int64
	err = h.retrieve(ctx, u.String(), "Authorization", client, func(resp *http.Response) error {
		if byteLimit <= 0 || resp.ContentLength > byteLimit {
			return &SourceError{Stage: "dataset download size limit"}
		}
		if err := file.Truncate(0); err != nil {
			return &SourceError{Stage: "temporary dataset storage"}
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return &SourceError{Stage: "temporary dataset storage"}
		}
		var err error
		size, err = io.Copy(file, io.LimitReader(resp.Body, byteLimit+1))
		if err != nil || size > byteLimit {
			return &SourceError{Stage: "dataset download size or read"}
		}
		return nil
	})
	if err != nil {
		return nil, 0, 0, err
	}
	reportActivity(ctx, "parsing", "Decoding published dataset rows")
	observations, rows, err := h.decodeParquet(ctx, file, size, rowLimit, dataset)
	return observations, size, rows, err
}

func (h *huggingFace) decodeParquet(ctx context.Context, file io.ReaderAt, size, rowLimit int64, dataset Dataset) (observations []Observation, count int64, err error) {
	// A malformed third-party schema must fail the refresh, not the gateway.
	defer func() {
		if recover() != nil {
			observations, count, err = nil, 0, &SourceError{Stage: "Parquet schema decoding"}
		}
	}()
	f, err := parquet.OpenFile(file, size)
	if err != nil {
		return nil, 0, &SourceError{Stage: "Parquet file parsing"}
	}
	if f.NumRows() <= 0 || f.NumRows() > rowLimit {
		return nil, 0, &SourceError{Stage: "dataset row limit or empty dataset"}
	}
	for _, group := range f.Metadata().RowGroups {
		if group.TotalByteSize < 0 || group.TotalByteSize > maxDatasetBytes {
			return nil, 0, &SourceError{Stage: "Parquet row group size limit"}
		}
	}
	reader := parquet.NewReader(f)
	defer reader.Close()
	batch := make([]parquet.Row, 128)
	for {
		if ctx.Err() != nil {
			return nil, 0, &SourceError{Stage: "dataset decoding cancellation"}
		}
		n, readErr := reader.ReadRows(batch)
		for _, row := range batch[:n] {
			fields := make(map[string]any)
			if err := f.Schema().Reconstruct(&fields, row); err != nil {
				return nil, 0, &SourceError{Stage: "Parquet row decoding"}
			}
			// Preserve the JSON row contract, including numeric and nested metadata.
			data, err := json.Marshal(fields)
			if err != nil {
				return nil, 0, &SourceError{Stage: "Parquet row values"}
			}
			if err := json.Unmarshal(data, &fields); err != nil {
				return nil, 0, &SourceError{Stage: "Parquet row values"}
			}
			parsed, err := h.parseRow(dataset, fields)
			if err != nil {
				return nil, 0, err
			}
			observations = append(observations, parsed...)
		}
		count += int64(n)
		if readErr == io.EOF {
			break
		}
		if readErr != nil || n == 0 {
			return nil, 0, &SourceError{Stage: "Parquet row reading"}
		}
	}
	if count != f.NumRows() {
		return nil, 0, &SourceError{Stage: "incomplete Parquet dataset"}
	}
	return observations, count, nil
}
