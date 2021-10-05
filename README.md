# go-websizer

Converts an image file into various WebP images to use with [`img srcset`](https://developer.mozilla.org/en-US/docs/Learn/HTML/Multimedia_and_embedding/Responsive_images).

## Install

```
$ go get github.com/pipe01/go-websizer
```

## Usage

```
Usage of go-websizer:
  -lossless
        whether to encode webp in lossless mode
  -parallel int
        maximum number of images to process in parallel (default 8)
  -quality float
        quality to use when encoding into webp (default 80)
  -quiet
        if true, only errors will be printed
  -size value
        comma-separated list of heights (default 480,720,1080)
```

### Examples

```
go-websizer image1.jpg image2.jpg
```
```
go-websizer -size 480,720 image*.jpg
```
