package handlers

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"github.com/minio/minio-go"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/nkonev/blog-storage/utils"
	"github.com/spf13/viper"
	"net/http"
	"net/url"
	"strconv"
	"syscall"
)

func NewFsHandler(minio *minio.Client, serverUrl string, client *mongo.Client) *FsHandler {
	return &FsHandler{minio: minio, serverUrl: serverUrl, mongo: client}
}

type FsHandler struct {
	serverUrl string
	minio     *minio.Client
	mongo     *mongo.Client
}

type FileInfo struct {
	Filename  string `json:"filename"`
	Url       string `json:"url"`
	PublicUrl string `json:"publicUrl"`
	Size      int64  `json:"size"`
}

func (h *FsHandler) LsHandler(c echo.Context) error {
	log.Debugf("Get userId: %v; userLogin: %v", c.Get(utils.USER_ID), c.Get(utils.USER_LOGIN))

	bucket := h.ensureAndGetBucket(c)
	// Create a done channel.
	doneCh := make(chan struct{})
	defer close(doneCh)

	log.Debugf("Listing bucket '%v':", bucket)

	var list []FileInfo = make([]FileInfo, 0)
	for objInfo := range h.minio.ListObjects(bucket, "", false, doneCh) {
		log.Debugf("Object '%v'", objInfo.Key)

		var downloadUrl *url.URL
		downloadUrl, err := url.Parse(h.serverUrl)
		if err != nil {
			return err
		}
		downloadUrl.Path += utils.DOWNLOAD_PREFIX + objInfo.Key

		published, err := h.isPublished(bucket, objInfo.Key)
		if err != nil {
			return err
		}
		publicUrl := ""
		if published {
			publicUrl = h.getPublicUrl(bucket, objInfo.Key)
		}
		info := FileInfo{Filename: objInfo.Key, Url: downloadUrl.String(), Size: objInfo.Size, PublicUrl: publicUrl}
		list = append(list, info)
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "files": list})
}

const FormFile = "file"

func (h *FsHandler) UploadHandler(c echo.Context) error {

	file, err := c.FormFile(FormFile)
	if err != nil {
		log.Errorf("Error during extracting form %v parameter: %v", FormFile, err)
		return err
	}

	bucketName := h.ensureAndGetBucket(c)

	// check limit
	consumption := h.calcUserFilesConsumption(bucketName)
	userId, ok := c.Get(utils.USER_ID).(int)
	if !ok {
		log.Errorf("Error during get(cast) userId")
	}
	maxAllowed, err := h.getMaxAllowedConsumption(userId)
	if err != nil {
		log.Errorf("Error during calculating max allowed %v", err)
		return err
	}
	if consumption+file.Size > maxAllowed {
		log.Infof("Upload too large %v+%v>%v bytes", consumption, file.Size, maxAllowed)
		return c.JSON(http.StatusRequestEntityTooLarge, &utils.H{"status": "fail"})
	}

	contentType := file.Header.Get("Content-Type")

	log.Debugf("Determined content type: %v", contentType)

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

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
	return getBucketNameInt(i)
}

func getBucketNameInt(userId interface{}) string {
	return fmt.Sprintf(utils.USER_PREFIX+"%v", userId)
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
			log.Debugf("Bucket '%s' already present", bucketName)
		} else {
			log.Fatal(err)
		}
	} else {
		log.Infof("Successfully created bucket '%s'", bucketName)
	}

}

func (h *FsHandler) download(bucketName, objName string) func(c echo.Context) error {
	return func(c echo.Context) error {
		info, e := h.minio.StatObject(bucketName, objName, minio.StatObjectOptions{})
		if e != nil {
			return c.JSON(http.StatusNotFound, &utils.H{"status": "stat fail"})
		}

		c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(info.Size, 10))
		c.Response().Header().Set(echo.HeaderContentType, info.ContentType)

		object, e := h.minio.GetObject(bucketName, objName, minio.GetObjectOptions{})
		defer object.Close()
		if e != nil {
			return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
		}

		return c.Stream(http.StatusOK, info.ContentType, object)
	}
}

func (h *FsHandler) DownloadHandler(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	objName := getFileName(c)

	return h.download(bucketName, objName)(c)
}

func getPublishDocument(objName string) bson.D {
	return bson.D{{"_id", objName}}
}

func (h *FsHandler) PublicDownloadHandler(c echo.Context) error {
	database := utils.GetMongoDatabase(h.mongo)

	bucketName := getBucketNameInt(c.Param("userId"))

	objName := getFileName(c)
	objName, err := url.PathUnescape(objName)
	if err != nil {
		return err
	}

	findResult := database.Collection(bucketName).FindOne(context.TODO(), getPublishDocument(objName))
	if findResult.Err() != nil {
		log.Errorf("Error during querying record from mongo")
		return findResult.Err()
	}
	err = findResult.Decode(nil)

	if err == mongo.ErrNoDocuments {
		return c.JSON(http.StatusNotFound, &utils.H{"status": "access fail"})
	}

	return h.download(bucketName, objName)(c)
}

func getFileName(context echo.Context) string {
	return context.Param("file")
}

func (h *FsHandler) MoveHandler(c echo.Context) error {
	from := c.Param("from")
	to := c.Param("to")
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
	objName, err := url.PathUnescape(objName)
	if err != nil {
		return err
	}
	if err := h.minio.RemoveObject(bucketName, objName); err != nil {
		log.Errorf("Error during remove object: %v", err)
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func (h *FsHandler) Limits(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	userId, ok := c.Get(utils.USER_ID).(int)
	if !ok {
		log.Errorf("Error during get(cast) userId")
	}

	max, e := h.getMaxAllowedConsumption(userId)
	if e != nil {
		return e
	}
	consumption := h.calcUserFilesConsumption(bucketName)

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "used": h.calcUserFilesConsumption(bucketName), "available": max - consumption})
}

func (h *FsHandler) calcUserFilesConsumption(bucketName string) int64 {
	var totalBucketConsumption int64

	recursive := true
	doneCh := make(chan struct{})
	defer close(doneCh)

	log.Debugf("Listing bucket '%v':", bucketName)
	for objInfo := range h.minio.ListObjects(bucketName, "", recursive, doneCh) {
		totalBucketConsumption += objInfo.Size
	}
	return totalBucketConsumption
}

func (h *FsHandler) getPublicUrl(bucketName, objName string) string {
	return h.serverUrl + utils.PUBLIC_PREFIX + "/" + bucketName + "/" + objName
}

func (h *FsHandler) Publish(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	objName := getFileName(c)
	objName, err := url.PathUnescape(objName)
	if err != nil {
		return err
	}
	_, e := h.minio.StatObject(bucketName, objName, minio.StatObjectOptions{})
	if e != nil {
		return c.JSON(http.StatusNotFound, &utils.H{"status": "stat fail"})
	}

	database := utils.GetMongoDatabase(h.mongo)

	upsert := true
	_, err2 := database.Collection(bucketName).UpdateOne(context.TODO(), getPublishDocument(objName), bson.D{}, &options.UpdateOptions{Upsert: &upsert})
	if err2 != nil {
		log.Errorf("Error during publishing '%v' : %v", objName, err)
		return err2
	}
	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "published": true, "url": h.getPublicUrl(getBucketName(c), objName)})
}

func (h *FsHandler) isDocumentExists(collection string, request interface{}, opts ...*options.FindOneOptions) (bool, error) {
	database := utils.GetMongoDatabase(h.mongo)

	// https://siongui.github.io/2017/03/13/go-pass-slice-or-array-as-variadic-parameter/#id12
	res := database.Collection(collection).FindOne(context.TODO(), request, opts[:]...)
	if res.Err() != nil {
		log.Errorf("Error during find '%v' : %v", request, res.Err())
		return false, res.Err()
	}

	_, e := res.DecodeBytes()

	if e != nil {
		if e == mongo.ErrNoDocuments {
			return false, nil
		} else {
			log.Errorf("Error during DecodeBytes '%v' : %v", request, res.Err())
			return false, e
		}
	} else {
		return true, nil
	}

}

func (h *FsHandler) isPublished(bucketName, objName string) (bool, error) {
	return h.isDocumentExists(bucketName, getPublishDocument(objName), &options.FindOneOptions{})
}

func (h *FsHandler) DeletePublish(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	objName := getFileName(c)
	objName, err := url.PathUnescape(objName)
	if err != nil {
		return err
	}

	database := utils.GetMongoDatabase(h.mongo)
	_, err = database.Collection(bucketName).DeleteOne(context.TODO(), getPublishDocument(objName), &options.DeleteOptions{})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "unpublished": true})
}

func (h *FsHandler) getMaxAllowedConsumption(userId int) (int64, error) {
	b, e := h.isDocumentExists("limits", bson.D{{"_id", userId}})
	if e != nil {
		return 0, e
	}

	if b {
		var stat syscall.Statfs_t
		wd := viper.GetString("limits.stat.dir")
		err := syscall.Statfs(wd, &stat)
		if err != nil {
			return 0, err
		}
		// Available blocks * size per block = available space in bytes
		u := int64(stat.Bavail * uint64(stat.Bsize))
		return u, nil
	} else {
		return viper.GetInt64("limits.default.per.user.max"), nil
	}
}
