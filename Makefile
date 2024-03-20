TARGET := rl

fmt:
	go fmt ./...

vet:
	go vet ./...

.PHONY: $(TARGET)

build: $(TARGET)

$(TARGET):
	go build -o $@ main.go

clean:
	rm -f $(TARGET) *~
