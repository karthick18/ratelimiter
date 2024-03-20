TARGET := ratelimiter

fmt:
	go fmt ./...

vet:
	go vet ./...

build: $(TARGET)

$(TARGET):
	go build -o $@ main.go

clean:
	rm -f $(TARGET) *~
