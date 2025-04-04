# build stage
## import image
FROM golang:alpine AS build_base

## add git
RUN apk add --no-cache git

## cd into global 700 dir (image specific)
WORKDIR /go

## populate the module cache based on the go.{mod,sum} files.
COPY go.mod .
COPY go.sum .
## download the modules
RUN go mod download

## copy all files into build stage
COPY . .
## BUILD BUILD BUILD!
RUN go build -o ./out/gosh .


# run stage
## import image
FROM alpine:latest
## install certificates
RUN apk add ca-certificates

## copy app from build stage to run stage
COPY --from=build_base /go/out/gosh /app/gosh

## copy files from build stage to run stage
COPY --from=build_base /go/* /app

## expose port
EXPOSE 8080
## run app
CMD ["/app/gosh", "-config", "/app/gosh.yml"]
