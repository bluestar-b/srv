NAME := srv
VERSION := "1.0.0"  # Set a static version here

GO_BUILDFLAGS := -trimpath
GO_LDFLAGS := -ldflags "-s -w -X main.VERSION=$(VERSION)"
GO_LDFLAGS_DEBUG := -ldflags "-X main.VERSION=$(VERSION)-DEBUG"
GO_LDFLAGS_STATIC := -tags netgo -ldflags "-s -w -X main.VERSION=$(VERSION) -extldflags -static"

.PHONY: build debug fmt clean release

build: clean fmt $(NAME)

$(NAME): main.go
		go build $(GO_BUILDFLAGS) -o $@ $(GO_LDFLAGS) .

debug: $(NAME)-debug
$(NAME)-debug: main.go
		go build $(GO_BUILDFLAGS) -o $@ -gcflags="all=-N -l" $(GO_LDFLAGS_DEBUG) .

fmt:
		go fmt

clean:
		rm -f $(NAME) $(NAME)-debug
		rm -rf release

# release static crossbuilds
define buildrelease
GOOS=$(1) GOARCH=$(2) go build $(GO_BUILDFLAGS) \
         -a \
         -o release/$(NAME)-$(1)-$(2) \
         $(GO_LDFLAGS_STATIC) . ;
upx -9 release/$(NAME)-$(1)-$(2);
sha512sum release/$(NAME)-$(1)-$(2) > release/$(NAME)-$(1)-$(2).sha512sum;
endef

GOOSARCHES = linux/arm linux/arm64 linux/amd64 darwin/amd64

release: main.go
		$(foreach GOOSARCH,$(GOOSARCHES), $(call buildrelease,$(subst /,,$(dir $(GOOSARCH))),$(notdir $(GOOSARCH))))

