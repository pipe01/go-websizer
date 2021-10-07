package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
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
	quality   = flag.Float64("quality", 80, "quality to use when encoding into webp or jpeg")
	lossless  = flag.Bool("lossless", false, "whether to encode webp in lossless mode")
	parallel  = flag.Int64("parallel", int64(runtime.NumCPU()), "maximum number of images to process in parallel")
	quiet     = flag.Bool("quiet", false, "if true, only errors will be printed")
	outFolder = flag.String("outDir", "", "folder to store output files on, by default they will be stored besides the original file")

	sizes = []Size{{480, defaultFormat}, {720, defaultFormat}, {1080, defaultFormat}}
)

const defaultFormat = "webp"

func main() {
	flag.Func("size", "comma-separated list of size-format (default 480-webp,720-webp,1080-webp)", func(s string) error {
		parts := strings.Split(s, ",")
		sizes = make([]Size, len(parts))

		for i, p := range parts {
			s, err := parseSize(p)
			if err != nil {
				return err
			}

			sizes[i] = s
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
			log.Printf("resizing image %s with size %d", path, size.Height)
		}

		var dir string
		if *outFolder == "" {
			dir = filepath.Dir(path)
		} else {
			dir = *outFolder
		}
		base := filepath.Join(dir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))

		if size.Height == 0 {
			newimg = img
			newpath = fmt.Sprintf("%s.%s", base, size.Format)
		} else {
			neww, newh := calcSize(w, h, size.Height)

			newimg = imaging.Resize(img, neww, newh, imaging.Lanczos)
			newpath = fmt.Sprintf("%s-%dp.%s", base, size.Height, size.Format)
		}

		out, err := os.Create(newpath)
		if err != nil {
			return fmt.Errorf("create file %s: %w", newpath, err)
		}
		defer out.Close() // Just in case

		if err := encode(out, newimg, size.Format); err != nil {
			return fmt.Errorf("encode file %s: %w", newpath, err)
		}

		out.Close()
	}

	return nil
}

func calcSize(w, h, newh int) (int, int) {
	return int((float32(w) / float32(h)) * float32(newh)), newh
}

func encode(w io.Writer, img image.Image, format string) error {
	switch format {
	case "webp":
		return webp.Encode(w, img, &webp.Options{Lossless: *lossless, Quality: float32(*quality)})
	case "jpeg", "jpg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: int(*quality)})
	case "png":
		return png.Encode(w, img)
	}

	return fmt.Errorf("unknown format %s", format)
}

type Size struct {
	Height int
	Format string
}

func parseSize(str string) (Size, error) {
	dash := strings.IndexRune(str, '-')

	if dash == -1 {
		size, err := strconv.Atoi(str)
		if err != nil {
			return Size{}, fmt.Errorf("parse %s: %w", str, err)
		}

		return Size{size, defaultFormat}, nil
	}

	size, err := strconv.Atoi(str[:dash])
	if err != nil {
		return Size{}, fmt.Errorf("parse %s: %w", str[:dash], err)
	}

	return Size{Height: size, Format: str[dash+1:]}, nil
}
