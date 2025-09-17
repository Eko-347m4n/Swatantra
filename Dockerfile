# --- Build Stage ---
# Menggunakan image Go resmi sebagai basis untuk build
FROM golang:1.21-alpine AS builder

# Set direktori kerja di dalam container
WORKDIR /app

# Copy file go.mod dan go.sum untuk men-download dependensi
COPY go.mod go.sum ./

# Download dependensi
RUN go mod download

# Copy seluruh source code proyek
COPY . .

# Build binary aplikasi, targetkan paket main di cmd/node
# -ldflags "-w -s" untuk mengurangi ukuran binary
# CGO_ENABLED=0 untuk static build
RUN CGO_ENABLED=0 go build -v -ldflags="-w -s" -o /swatantra-node ./cmd/node

# --- Final Stage ---
# Menggunakan image scratch yang kosong untuk hasil akhir yang minimal
FROM scratch

# Copy binary yang sudah di-build dari stage sebelumnya
COPY --from=builder /swatantra-node /swatantra-node

# (Opsional) Copy file konfigurasi default jika diperlukan saat runtime
# Jika tidak ada, node mungkin akan membuat file default atau memerlukan flag
COPY config/config.json /config/config.json

# Expose port default untuk P2P dan API jika ada
# Ini hanya untuk dokumentasi, tidak membuka port secara otomatis
# EXPOSE 3000
# EXPOSE 4000

# Set binary sebagai entrypoint container
ENTRYPOINT ["/swatantra-node"]
