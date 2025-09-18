# Swatantra

Swatantra adalah proyek blockchain yang dirancang untuk efisiensi dan keamanan, dengan memanfaatkan model UTXO, Ed25519 untuk tanda tangan digital, dan Keccak256 untuk hashing. Proyek ini mengimplementasikan mekanisme konsensus Proof of Work (PoW) dengan penyesuaian kesulitan (difficulty) berbasis Exponential Moving Average (EMA) dan aturan pemilihan fork yang kuat.

## Daftar Isi

- [Fitur Utama](#fitur-utama)
- [Instalasi &amp; Penggunaan](#instalasi--penggunaan)
- [Konfigurasi](#konfigurasi)
- [Pengujian](#pengujian)
- [Deployment dengan Docker](#deployment-dengan-docker)
- [Pipeline CI/CD](#pipeline-ci--cd)
- [Kontribusi](#kontribusi)

## Fitur Utama

- **Model UTXO**: Menyediakan cara yang sederhana dan aman untuk melacak status transaksi.
- **Kriptografi Modern**: Menggunakan Ed25519 untuk tanda tangan dan Keccak256 untuk hashing.
- **Konsensus Proof of Work**: Diamankan oleh algoritma PoW dengan penyesuaian tingkat kesulitan yang dinamis.
- **Jaringan P2P**: Node dapat saling menemukan, terhubung, dan melakukan sinkronisasi satu sama lain.
- **Antarmuka CLI**: Sebuah antarmuka baris perintah (CLI) untuk build, menjalankan node, dan membuat wallet.

## Instalasi & Penggunaan

### Prasyarat

- **Go**: Versi 1.24 atau lebih tinggi.

### 1. Build dari Kode Sumber

Dari direktori utama proyek, jalankan perintah berikut untuk membuat file aplikasi `swatantra-node`:

```bash
go build -v -o build/swatantra-node ./cmd/node
```

Aplikasi akan dibuat di dalam direktori `build`.

### 2. Membuat Wallet

Perintah ini akan membuat file `wallet.key` di direktori Anda saat ini. **Simpan file ini dengan aman!**

```bash
./build/swatantra-node create-wallet
```

### 3. Menjalankan Node

Jalankan node menggunakan konfigurasi default. Untuk mengaktifkan mining, tambahkan flag `--mine` dan sebutkan alamat wallet Anda untuk menerima hadiah.

```bash
# Menjalankan node biasa
./build/swatantra-node start-node

# Menjalankan node dengan mining aktif
./build/swatantra-node start-node --mine --coinbase ALAMAT_WALLET_ANDA
```

## Konfigurasi

Node menggunakan file `config.json` untuk konfigurasi. Contoh file konfigurasi sudah disediakan di `config/config.json`. Anda dapat mengubah port P2P/API, daftar peer awal, dan parameter chain lainnya.

**Contoh `config.json`:**

```json
{
  "p2p": {
    "listenAddress": ":3000"
  },
  "api": {
    "listenAddress": ":4000"
  }
}
```

## Pengujian

Proyek ini menyertakan serangkaian pengujian unit dan integrasi yang lengkap. Untuk menjalankan semua pengujian, gunakan perintah berikut dari direktori utama proyek:

```bash
go test -v ./...
```

Perintah ini akan secara otomatis membangun dan menjalankan beberapa node di memori untuk memverifikasi skenario kompleks seperti sinkronisasi chain dan fork blockchain, demi memastikan stabilitas logika jaringan.

## Deployment dengan Docker

Instalasi dan menjalankan node Swatantra menggunakan Docker adalah cara yang paling direkomendasikan karena memastikan lingkungan yang konsisten dan terisolasi.

### Prasyarat Docker

- **Docker**: Pastikan Docker sudah terinstal dan berjalan di sistem Anda.

### Langkah 1: Dapatkan Docker Image

Ada dua cara untuk mendapatkan image Docker Swatantra:

**Opsi A (Disarankan): Tarik dari Docker Hub**

```bash
docker pull 347m4n/swatantra-node:latest
```

**Opsi B: Build dari Kode Sumber**

Jika Anda memiliki kode sumber dan ingin membuat image sendiri, jalankan perintah ini dari root direktori proyek:

```bash
docker build -t 347m4n/swatantra-node .
```

### Langkah 2: Siapkan Direktori & Konfigurasi Lokal

Container Docker berjalan terisolasi. Agar data (blockchain, wallet, config) tidak hilang saat container dimatikan, kita perlu menyimpannya di mesin lokal Anda.

1. **Buat Direktori Data**

   ```bash
   mkdir -p swatantra-data/config
   ```

2. **Salin File Konfigurasi**

   ```bash
   cp config/config.json swatantra-data/config/
   ```

3. **Buat Wallet**
   Gunakan image Docker untuk membuat `wallet.key` di dalam direktori data Anda.

   ```bash
   docker run --rm -v "$(pwd)/swatantra-data:/data" 347m4n/swatantra-node:latest create-wallet --datadir /data
   ```

   Anda sekarang akan memiliki file `wallet.key` di dalam folder `swatantra-data`.

### Langkah 3: Jalankan Node Swatantra

Perintah ini akan memulai container, menghubungkan direktori data Anda, dan memetakan port jaringan.

```bash
docker run -d \
  --name swatantra-node-1 \
  -p 3000:3000 \
  -p 4000:4000 \
  -v "$(pwd)/swatantra-data:/data" \
  347m4n/swatantra-node:latest \
  start-node --datadir /data --config /data/config/config.json
```

- `docker run -d`: Menjalankan container di background.
- `--name swatantra-node-1`: Memberi nama container agar mudah dikelola.
- `-p 3000:3000`: Memetakan port P2P.
- `-p 4000:4000`: Memetakan port API.
- `-v "$(pwd)/swatantra-data:/data"`: Menghubungkan direktori data lokal ke direktori `/data` di dalam container. **Ini adalah langkah paling penting untuk persistensi data.**

### Langkah 4: Berinteraksi dengan Node

- **Melihat Log Node:**

  ```bash
  docker logs -f swatantra-node-1
  ```

- **Memeriksa Status via API:**

  ```bash
  curl http://localhost:4000/status
  ```

- **Menghentikan Node:**

  ```bash
  docker stop swatantra-node-1
  ```

- **Memulai Node Kembali:**

  ```bash
  docker start swatantra-node-1
  ```

## Pipeline CI & CD

Proyek ini menggunakan GitHub Actions (`.github/workflows/pub.yml`) untuk pipeline CI/CD-nya, yang mengotomatiskan hal-hal berikut:

- **Saat Pull Request ke `main`**: Membangun kode dan menjalankan semua pengujian.
- **Saat Push ke `main`**: Membangun kode, menjalankan tes, dan mendorong image Docker `:latest` ke Docker Hub.
- **Saat ada Tag Versi (misal: `v0.1.0`)**:
  1. Membangun kode dan menjalankan tes.
  2. Membuat **GitHub Release** baru.
  3. Mengkompilasi dan melampirkan aplikasi `swatantra-node-linux` ke rilis tersebut.
  4. Membangun dan mendorong image Docker dengan tag versi (`:v0.1.0`) dan `:latest` ke Docker Hub.

## Kontribusi

Kontribusi untuk proyek Swatantra sangat diharapkan! Jika Anda menemukan bug atau memiliki ide untuk fitur baru, silakan buat _Issue_ atau _Pull Request_.
