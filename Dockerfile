FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27.3-alpine AS build
ARG AUMBRY_VERSION=dev
ARG AUMBRY_COMMIT=none
ARG AUMBRY_BUILD_DATE=
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /src/web/dist/ ./internal/webembed/dist/
RUN mkdir -p /data && chown 65532:65532 /data
RUN BUILD_DATE="${AUMBRY_BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
    && LDFLAGS="-s -w -X github.com/nerdneilsfield/aumbry/internal/buildinfo.Version=${AUMBRY_VERSION} -X github.com/nerdneilsfield/aumbry/internal/buildinfo.Commit=${AUMBRY_COMMIT} -X github.com/nerdneilsfield/aumbry/internal/buildinfo.Date=${BUILD_DATE}" \
    && CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o /out/aumbry . \
    && CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o /out/aumbry-server ./cmd/server

FROM gcr.io/distroless/static-debian13:nonroot
WORKDIR /app
COPY --from=build --chown=65532:65532 /data /data
COPY --from=build /out/aumbry /usr/local/bin/aumbry
COPY --from=build /out/aumbry-server /usr/local/bin/aumbry-server
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/aumbry-server"]
