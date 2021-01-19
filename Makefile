export ${HOME}
GOCARD = ${HOME}/go/src/github.com/adakailabs/gocard

${GOCARD}:
	go get -d github.com/adakailabs/gocard


install: ${GOCARD}
	go install gocard.go

.PHONY: test
test:
	@echo "hello ${GOCARD}"
