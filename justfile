binary := 'cyclaw'
version := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
ld_flags := "-trimpath -ldflags '-s -w -X main.version=" + version + "'"
go_build := 'CGO_ENABLED=0 go build ' + ld_flags

build:
  mkdir -p build
  {{ go_build }} -o build/{{binary}} .

clean:
  rm -rf build/

test:
  go test -v ./...

lint:
  go vet ./...

docker:
  docker build -t {{ binary }}:{{ version }} .