FROM golang:1.22 as builder
WORKDIR /go/src/app
COPY . .

RUN go mod download
RUN go vet -v
RUN go test -v

RUN CGO_ENABLED=0 go build -o /go/bin/gosh

FROM alpine
COPY --from=builder /go/bin/gosh /
COPY --from=builder /go/src/app/gosh.yml /gosh.yml
RUN adduser "_gosh" -D
RUN mkdir /store
EXPOSE 8080
CMD ["/gosh" ,"-config","gosh.yml"]
