package handlers

import (
	"github.com/labstack/echo"
	"github.com/labstack/gommon/log"
	"github.com/minio/minio-go"
	"github.com/nkonev/blog-store/utils"
	"net/http"
)

func NewFsHandler(minio *minio.Client) *FsHandler {
	return &FsHandler{minio: minio}
}

type FsHandler struct {
	minio *minio.Client
}

func (h *FsHandler) LsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func (h *FsHandler) UploadHandler(c echo.Context) error {

	file, err := c.FormFile("file")
	if err != nil {
		return err
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	bucketName := h.ensureAndGetBucket(c)

	contentType := file.Header.Get("Content-Type")

	log.Infof("Determined content type: %v", contentType)

	if _, err := h.minio.PutObject(bucketName, file.Filename, src, file.Size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func getBucketName(c echo.Context) string {
	return "temporary-todo"
}

func getBucketLocation(c echo.Context) string {
	return "europe-east"
}

func (h *FsHandler) ensureAndGetBucket(c echo.Context) string {
	bucketName := getBucketName(c)
	bucketLocation := getBucketLocation(c)
	h.ensureBucket(bucketName, bucketLocation)
	return bucketName
}

func (h *FsHandler) ensureBucket(bucketName, location string) {
	err := h.minio.MakeBucket(bucketName, location)
	if err != nil {
		// Check to see if we already own this bucket (which happens if you run this twice)
		exists, err := h.minio.BucketExists(bucketName)
		if err == nil && exists {
			log.Printf("We already own %s", bucketName)
		} else {
			log.Fatal(err)
		}
	} else {
		log.Printf("Successfully created %s", bucketName)
	}

}

func (h *FsHandler) DownloadHandler(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	objName := getFileName(c)

	info, e := h.minio.StatObject(bucketName, objName, minio.StatObjectOptions{})
	if e != nil {
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "stat fail"})
	}

	object, e := h.minio.GetObject(bucketName, objName, minio.GetObjectOptions{})
	if e != nil {
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}

	defer object.Close()

	return c.Stream(http.StatusOK, info.ContentType, object)
}

func getFileName(context echo.Context) string {
	return context.Param("file")
}

func (h *FsHandler) MoveHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func (h *FsHandler) DeleteHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}
