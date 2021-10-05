package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"golang.org/x/sync/semaphore"
)

var (
	quality  = flag.Float64("quality", 80, "quality to use when encoding into webp")
	lossless = flag.Bool("lossless", false, "whether to encode webp in lossless mode")
	parallel = flag.Int64("parallel", int64(runtime.NumCPU()), "maximum number of images to process in parallel")
	quiet    = flag.Bool("quiet", false, "if true, only errors will be printed")

	sizes = []int{480, 720, 1080}
)

func main() {
	flag.Func("size", "comma-separated list of heights (default 480,720,1080)", func(s string) error {
		parts := strings.Split(s, ",")
		sizes = make([]int, len(parts))

		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				return fmt.Errorf("parse %s: %w", p, err)
			}

			sizes[i] = n
		}

		return nil
	})
	flag.Parse()

	files := make([]string, 0, flag.NArg())
	for _, f := range flag.Args() {
		fs, err := filepath.Glob(f)
		if err != nil {
			log.Fatalf("failed to glob files: %s", f)
		}

		files = append(files, fs...)
	}

	wg := sync.WaitGroup{}
	sem := semaphore.NewWeighted(*parallel)
	start := time.Now()

	for _, f := range files {
		wg.Add(1)

		go func(f string) {
			sem.Acquire(context.Background(), 1)
			defer sem.Release(1)
			defer wg.Done()

			if err := process(f); err != nil {
				log.Fatalf("failed to resize image: %s", err)
			}
		}(f)
	}

	wg.Wait()

	end := time.Now()
	if !*quiet {
		log.Printf("done in %s", end.Sub(start))
	}
}

func process(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer in.Close()

	img, _, err := image.Decode(in)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}

	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	for _, size := range sizes {
		var newimg image.Image
		var newpath string

		if !*quiet {
			log.Printf("resizing image %s with size %d", path, size)
		}

		if size == 0 {
			newimg = img
			newpath = fmt.Sprintf("%s.webp", strings.TrimSuffix(path, filepath.Ext(path)))
		} else {
			neww, newh := calcSize(w, h, size)

			newimg = imaging.Resize(img, neww, newh, imaging.Lanczos)
			newpath = fmt.Sprintf("%s-%dp.webp", strings.TrimSuffix(path, filepath.Ext(path)), size)
		}

		out, err := os.Create(newpath)
		if err != nil {
			return fmt.Errorf("create file %s: %w", newpath, err)
		}
		defer out.Close() // Just in case

		if err := webp.Encode(out, newimg, &webp.Options{Lossless: *lossless, Quality: float32(*quality)}); err != nil {
			return fmt.Errorf("encode file %s: %w", newpath, err)
		}

		out.Close()
	}

	return nil
}

func calcSize(w, h, newh int) (int, int) {
	return int((float32(w) / float32(h)) * float32(newh)), newh
}
