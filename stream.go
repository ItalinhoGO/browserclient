package browserclient

import (
	"bufio"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"compress/zlib"
)

func StreamResponse(resp *http.Response, config *StreamConfig) (*StreamResult, error) {
	if config.BufferSize <= 0 {
		config.BufferSize = 8192
	}

	reader, err := getResponseReader(resp)
	if err != nil {
		return nil, err
	}

	countingReader := &byteCountingReader{Reader: reader}
	bufReader := bufio.NewReaderSize(countingReader, config.BufferSize)

	var contentBuilder strings.Builder
	found := false
	stopContent := config.StopOnContent

	for {
		if config.MaxBytes > 0 && countingReader.BytesRead >= config.MaxBytes {
			break
		}

		line, err := bufReader.ReadString('\n')
		if line != "" {
			contentBuilder.WriteString(line)

			if stopContent != "" && strings.Contains(line, stopContent) {
				found = true
				break
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}

	return &StreamResult{
		BytesRead:    countingReader.BytesRead,
		Content:      contentBuilder.String(),
		FoundContent: found,
	}, nil
}

func getResponseReader(resp *http.Response) (io.Reader, error) {
	contentEncoding := resp.Header.Get("Content-Encoding")
	switch {
	case strings.Contains(contentEncoding, "gzip"):
		return gzip.NewReader(resp.Body)
	case strings.Contains(contentEncoding, "deflate"):
		return flateReader(resp.Body)
	default:
		return resp.Body, nil
	}
}

func flateReader(r io.Reader) (io.Reader, error) {
	return zlib.NewReader(r)
}

type byteCountingReader struct {
	io.Reader
	BytesRead int64
}

func (bcr *byteCountingReader) Read(p []byte) (int, error) {
	n, err := bcr.Reader.Read(p)
	bcr.BytesRead += int64(n)
	return n, err
}