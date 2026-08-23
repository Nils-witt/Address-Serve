FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/address-serv ./src

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/address-serv /address-serv
EXPOSE 8080
ENTRYPOINT ["/address-serv"]
