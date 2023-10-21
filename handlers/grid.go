package handlers

import (
	data "instafix/handlers/data"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

var transport = &http.Transport{
	MaxConnsPerHost: 1000,
}
var timeout = 10 * time.Second

func Grid() fiber.Handler {
	return func(c *fiber.Ctx) error {
		postID := c.Params("postID")

		// If already exists, return
		if _, err := os.Stat("static/" + postID + ".webp"); err == nil {
			return c.SendFile("static/" + postID + ".webp")
		}

		// Get data
		item := &data.InstaData{PostID: postID}
		err := item.GetData(postID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		// Only get first 4 images
		if len(item.Medias) == 1 {
			return c.Redirect("/images/" + postID + "/1")
		}
		mediaList := item.Medias[:min(4, len(item.Medias))]

		var images []*vips.ImageRef
		var wg sync.WaitGroup
		var mutex sync.Mutex
		wg.Add(len(mediaList))

		client := http.Client{Transport: transport, Timeout: timeout}
		for _, media := range mediaList {
			go func(media data.Media) {
				defer wg.Done()

				// Skip if not image
				if !strings.Contains(media.TypeName, "Image") {
					return
				}
				req, err := http.NewRequest(http.MethodGet, media.URL, nil)
				if err != nil {
					return
				}

				// Make request client.Get
				res, err := client.Do(req)
				if err != nil {
					return
				}
				defer res.Body.Close()
				buf, err := io.ReadAll(res.Body)

				image, err := vips.NewImageFromBuffer(buf)
				if err != nil {
					log.Error().Str("postID", postID).Err(err).Msg("Failed to create image from buffer")
					return
				}

				// Append image
				mutex.Lock()
				images = append(images, image)
				defer mutex.Unlock()
			}(media)
		}
		wg.Wait()

		if len(images) == 0 {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "no images found",
			})
		} else if len(images) == 1 {
			return c.Redirect("/images/" + postID + "/1")
		}

		// Join images
		stem := images[0]
		err = stem.ArrayJoin(images[1:], 2)
		if err != nil {
			log.Error().Str("postID", postID).Err(err).Msg("Failed to join images")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		// Export to static/ folder
		imagesBuf, _, err := stem.ExportWebp(nil)
		if err != nil {
			log.Error().Str("postID", postID).Err(err).Msg("Failed to export grid image")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		// SAVE imagesBuf to static/ folder
		f, err := os.Create("static/" + postID + ".webp")
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		defer f.Close()
		f.Write(imagesBuf)

		return c.SendFile("static/" + postID + ".webp")
	}
}