package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/ulikunitz/xz"
)

func init() {
	initLogger()
}

func main() {

	if len(os.Args) != 2 {
		slog.Error("provide exactly one argument, the path to the .jsonl.xz file to be parsed")
		os.Exit(1)
	}

	db, err := dbConnect()
	if err != nil {
		slog.Error("could not connect to database", "error", err)
		os.Exit(1)
	}

	path := os.Args[1]
	file, err := os.Open(path)
	if err != nil {
		slog.Error("could not open file", "file", file, "error", err)
		os.Exit(1)
	}

	decompressed, err := xz.NewReader(file)
	if err != nil {
		slog.Error("could not decompress file", "error", err)
		os.Exit(1)
	}

	var casesImported int64

	// Scan line by line, but into a large buffer in case there are large cases
	scanner := bufio.NewScanner(decompressed)
	maxSize := 1024 * 1000 * 1000 * 2 // 2 gigabytes
	buf := make([]byte, 0, maxSize)
	scanner.Buffer(buf, maxSize)
	for scanner.Scan() {
		// Read in one case per line
		c := &Case{}
		err = json.Unmarshal(scanner.Bytes(), c)
		if err != nil {
			slog.Error("can't unmarshall case JSON", "error", err)
			os.Exit(1)
		}
		err = c.Save(context.Background(), db)
		if err != nil {
			slog.Error("could not save case to database", "error", err)
			os.Exit(1)
		}
		casesImported += 1
	}
	if err := scanner.Err(); err != nil {
		slog.Error("could not scan file", "error", err)
		os.Exit(1)
	}

	slog.Info("imported cases", "num_cases", casesImported)

}
