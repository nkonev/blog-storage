docker run -p 9000:9000 -e "MINIO_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE" -e "MINIO_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" -v $(pwd)/data:/data minio/minio server /data






func Ls(what *string) error {
	bucket := ctx.Get("username")
}

func Upload(input http.Reader, filename string) error {
	bucket := ctx.Get("username")
}

func Download(output http.Writer, filename string) error {
	bucket := ctx.Get("username")
}

func Move(from, to string *) error {
	bucket := ctx.Get("username")
}

func Delete(what string) error {
	bucket := ctx.Get("username")
}

func handleAuthorize(ctx *Context){
	if success {
		ctx.Put("username", username)
	}
}

func main(){

}