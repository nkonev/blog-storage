```
docker run --name some-mongo -d -p27017:27017 mongo:4.1.7
docker run -p 9000:9000 -e "MINIO_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE" -e "MINIO_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" -v $(pwd)/data:/data -d minio/minio:RELEASE.2019-01-31T00-31-19Z server /data```