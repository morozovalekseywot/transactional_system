FROM golang:latest

RUN go version
ENV GOPATH=/

COPY ./ ./

# build go app
RUN go mod download
RUN go build -o transaction-app ./cmd/server/main.go

CMD ["./transaction-app"]