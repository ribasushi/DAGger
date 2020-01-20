package repacker

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
)

type Repacker struct {
	cfg config
}

type recordSizes []int64

type transformResult struct {
	recordSizes
	output     io.Reader
	outputSize *int64
	error      error
}

type PathTuple struct {
	Path  string
	Lstat os.FileInfo
}

func (rpk *Repacker) RecursePaths(pts []PathTuple) error {

	for _, pt := range pts {
		if pt.Lstat.Mode().IsRegular() {
			if err := rpk.processFile(pt); err != nil {
				return err
			}
		} else if pt.Lstat.IsDir() {
			dh, err := os.Open(pt.Path)
			if err != nil {
				log.Printf("skipping: %s\n", err)
				continue
			}

			// directory recursion never produces errors, just logs "skips"
			subPts := func() []PathTuple {
				defer dh.Close()

				dirContents, err := dh.Readdir(0)
				if err != nil {
					log.Printf("skipping: %s\n", err)
					return nil
				}

				if rpk.cfg.SortDirs {
					sort.Slice(dirContents, func(i, j int) bool {
						return dirContents[i].Name() < dirContents[j].Name()
					})
				}

				subPts := make([]PathTuple, len(dirContents))
				for i := range dirContents {
					subPts[i] = PathTuple{
						Path:  pt.Path + "/" + dirContents[i].Name(),
						Lstat: dirContents[i],
					}
				}
				return subPts
			}()

			if subPts != nil {
				if err := rpk.RecursePaths(subPts); err != nil {
					return err
				}
			}

		} else {
			// quiet for now
			//log.Printf("skipping: unsupported filemode %s for %s\n", pt.Lstat.Mode(), pt.Path)
		}
	}

	return nil
}

func (rpk *Repacker) processFile(pt PathTuple) error {

	fh, err := os.Open(pt.Path)
	if err != nil {
		return err
	}
	defer fh.Close()

	if err := binary.Write(os.Stdout, binary.BigEndian, pt.Lstat.Size()); err != nil {
		return fmt.Errorf("Error streaming out size prefix for %s: %s\n", pt.Path, err)
	}

	if _, err := io.Copy(os.Stdout, fh); err != nil {
		return fmt.Errorf("Error streaming out data for %s: %s\n", pt.Path, err)
	}

	return nil
}
