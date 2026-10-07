# 1. React app
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Go server with the built app embedded
FROM golang:1.26-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/portfolio ./cmd/portfolio

# 3. One static binary (+ seed content for `portfolio seed`)
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=server /out/portfolio /app/portfolio
COPY seed/data.json /app/seed/data.json
COPY seed/images /app/seed/images
ENV PORT=8080 APP_ENV=production
EXPOSE 8080
ENTRYPOINT ["/app/portfolio"]
