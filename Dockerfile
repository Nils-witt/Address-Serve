FROM --platform=$BUILDPLATFORM node:26-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist ./frontend/dist
# Build info shown in the web UI footer; the Docker workflow passes these.
ARG VERSION=dev
ARG COMMIT=none
ARG BRANCH=
RUN CGO_ENABLED=0 go build \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.branch=${BRANCH}" \
    -o /out/address-serv ./cmd/address-serv

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/address-serv /address-serv
EXPOSE 8080
ENTRYPOINT ["/address-serv"]
