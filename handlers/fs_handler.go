package handlers

import (
	"fmt"
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

type FileInfo struct {
	Filename string `json:"filename"`
}

func (h *FsHandler) LsHandler(c echo.Context) error {
	log.Infof("Get userId: %v; userLogin: %v", c.Get(utils.USER_ID), c.Get(utils.USER_LOGIN))

	bucket := h.ensureAndGetBucket(c)
	// Create a done channel.
	doneCh := make(chan struct{})
	defer close(doneCh)
	// Recurively list all objects in 'mytestbucket'
	recursive := true
	log.Infof("Listing bucket '%v':", bucket)

	var buffer []FileInfo = make([]FileInfo, 0)
	for objInfo := range h.minio.ListObjects(bucket, "", recursive, doneCh) {
		log.Infof("Object '%v'", objInfo.Key)

		buffer = append(buffer, FileInfo{Filename: objInfo.Key})
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "files": buffer})
}

const FormFile = "file"

func (h *FsHandler) UploadHandler(c echo.Context) error {

	file, err := c.FormFile(FormFile)
	if err != nil {
		log.Errorf("Error during extracting form %v parameter", FormFile)
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
		log.Errorf("Error during upload object: %v", err)
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func getBucketName(c echo.Context) string {
	i, ok := c.Get(utils.USER_ID).(int)
	if !ok {
		log.Errorf("Error during get(cast) userId")
	}
	return fmt.Sprintf("user%v", i)
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
			log.Printf("Bucket '%s' already present", bucketName)
		} else {
			log.Fatal(err)
		}
	} else {
		log.Printf("Successfully created bucket '%s'", bucketName)
	}

}

func (h *FsHandler) DownloadHandler(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	objName := getFileName(c)

	info, e := h.minio.StatObject(bucketName, objName, minio.StatObjectOptions{})
	if e != nil {
		return c.JSON(http.StatusNotFound, &utils.H{"status": "stat fail"})
	}

	object, e := h.minio.GetObject(bucketName, objName, minio.GetObjectOptions{})
	defer object.Close()
	if e != nil {
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}

	return c.Stream(http.StatusOK, info.ContentType, object)
}

func getFileName(context echo.Context) string {
	return context.Param("file")
}

func (h *FsHandler) MoveHandler(c echo.Context) error {
	from := c.Param("from")
	to := c.Param("to")
	// TODO make vfs in mongo
	bucketName := h.ensureAndGetBucket(c)

	info, e := h.minio.StatObject(bucketName, from, minio.StatObjectOptions{})
	if e != nil {
		return c.JSON(http.StatusNotFound, &utils.H{"status": "stat fail"})
	}

	object, err := h.minio.GetObject(bucketName, from, minio.GetObjectOptions{})
	defer object.Close()
	if err != nil {
		log.Errorf("Error during get object: %v", err)
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}

	if _, err := h.minio.PutObject(bucketName, to, object, info.Size, minio.PutObjectOptions{ContentType: info.ContentType}); err != nil {
		log.Errorf("Error during copy object: %v", err)
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func (h *FsHandler) DeleteHandler(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)
	objName := getFileName(c)
	if err := h.minio.RemoveObject(bucketName, objName); err != nil {
		log.Errorf("Error during remove object: %v", err)
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}
