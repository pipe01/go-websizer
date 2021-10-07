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
	parallel  = flag.Int("parallel", runtime.NumCPU(), "maximum number of images to process in parallel")
	quiet     = flag.Bool("quiet", false, "if true, only errors will be printed")
	outFolder = flag.String("outDir", "", "folder to store output files on, by default they will be stored besides the original file")
	ifNewer   = flag.Bool("ifNewer", false, "only encode an image if the output image doesn't exist or it's older than the original image")

	sizes = []Size{{480, defaultFormat}, {720, defaultFormat}, {1080, defaultFormat}}
	jobs  = make(chan *Job, 100)
)

type Job struct {
	img      image.Image
	size     Size
	outPath  string
	origPath string
}

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
	start := time.Now()

	for i := 0; i < *parallel; i++ {
		go func() {
			for job := range jobs {
				if err := doJob(job); err != nil {
					log.Fatalf("failed to process image: %s", err)
				}
				wg.Done()
			}
		}()
	}

	scanwg := sync.WaitGroup{}
	sem := semaphore.NewWeighted(int64(*parallel))
	for _, f := range files {
		scanwg.Add(1)
		go func(f string) {
			sem.Acquire(context.Background(), 1)
			if err := enqueue(f, &wg); err != nil {
				log.Fatalf("failed to resize image: %s", err)
			}
			sem.Release(1)
			scanwg.Done()
		}(f)
	}
	scanwg.Wait()
	close(jobs)

	wg.Wait()

	end := time.Now()
	if !*quiet {
		log.Printf("done in %s", end.Sub(start))
	}
}

func enqueue(path string, wg interface{ Add(int) }) error {
	in, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer in.Close()

	var img image.Image

	for _, size := range sizes {
		var newpath string

		var dir string
		if *outFolder == "" {
			dir = filepath.Dir(path)
		} else {
			dir = *outFolder
		}
		base := filepath.Join(dir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))

		if size.Height == 0 {
			newpath = fmt.Sprintf("%s.%s", base, size.Format)
		} else {
			newpath = fmt.Sprintf("%s-%dp.%s", base, size.Height, size.Format)
		}

		// Check if the output image is up to date
		if *ifNewer {
			outfi, err := os.Stat(newpath)
			if err == nil {
				srcfi, err := os.Stat(path)
				if err == nil && outfi.ModTime().After(srcfi.ModTime()) {
					if !*quiet {
						log.Printf("skipped image %s", newpath)
					}
					continue
				}
			}
		}

		// Lazy load image because we may not need to load it if all sizes are up to date
		if img == nil {
			img, _, err = image.Decode(in)
			if err != nil {
				return fmt.Errorf("decode image: %w", err)
			}
		}

		wg.Add(1)
		jobs <- &Job{
			img:      img,
			size:     size,
			outPath:  newpath,
			origPath: path,
		}
	}

	return nil
}

func doJob(job *Job) error {
	if !*quiet {
		log.Printf("resizing image %s with size %d encoded to %s", job.origPath, job.size.Height, job.size.Format)
	}

	w, h := job.img.Bounds().Dx(), job.img.Bounds().Dy()

	var newimg image.Image
	if job.size.Height == 0 {
		newimg = job.img
	} else {
		newimg = imaging.Resize(job.img, calcWidth(w, h, job.size.Height), job.size.Height, imaging.Lanczos)
	}

	os.MkdirAll(filepath.Dir(job.outPath), os.ModePerm)

	out, err := os.Create(job.outPath)
	if err != nil {
		return fmt.Errorf("create file %s: %w", job.outPath, err)
	}
	defer out.Close() // Just in case

	if err := encode(out, newimg, job.size.Format); err != nil {
		return fmt.Errorf("encode file %s: %w", job.outPath, err)
	}

	out.Close()
	return nil
}

func calcWidth(w, h, newh int) int {
	return int((float32(w) / float32(h)) * float32(newh))
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
