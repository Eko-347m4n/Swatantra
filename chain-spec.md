# Spesifikasi Protokol Blockchain Swatantra

Dokumen ini mendefinisikan spesifikasi teknis untuk Swatantra, sebuah blockchain yang dirancang untuk efisiensi dan keamanan.

## 1. Model Transaksi: UTXO (Unspent Transaction Output)

Swatantra menggunakan model **UTXO** untuk mengelola state. Setiap transaksi menghabiskan UTXO yang ada dan membuat UTXO baru.

- **State**: Kumpulan semua UTXO yang belum dihabiskan di seluruh blockchain.
- **Keuntungan**: Mendorong privasi (alamat dapat diganti di setiap transaksi) dan memungkinkan validasi transaksi secara paralel.
- **Implikasi**: Tidak ada konsep "saldo akun" di tingkat protokol; saldo dihitung oleh klien/wallet dengan menjumlahkan nilai semua UTXO yang dimiliki oleh sebuah kunci.

## 2. Algoritma Kriptografi

Protokol ini menggunakan algoritma standar industri untuk memastikan keamanan.

- **Tanda Tangan Digital**: **Ed25519**. Algoritma ini dipilih karena kecepatannya dalam verifikasi dan pembuatan tanda tangan, serta keamanan yang tinggi.
- **Fungsi Hash**: **Keccak256**. Digunakan untuk semua kebutuhan hashing, termasuk:
    - Menghasilkan ID transaksi.
    - Menghasilkan hash block.
    - Membangun Merkle Tree.

## 3. Struktur Data

### 3.1. Transaksi

Sebuah transaksi terdiri dari input, output, dan tanda tangan.

```json
{
  "txId": "string (hash dari body transaksi)",
  "inputs": [
    {
      "txId": "string (ID tx dari UTXO yang dihabiskan)",
      "outputIndex": "number (indeks output dalam tx sebelumnya)",
      "signature": "string (tanda tangan Ed25519)"
    }
  ],
  "outputs": [
    {
      "address": "string (alamat penerima, public key)",
      "value": "number (jumlah koin)"
    }
  ]
}
```

- `inputs`: Referensi ke UTXO yang akan dihabiskan. Setiap input harus ditandatangani oleh pemilik UTXO tersebut.
- `outputs`: UTXO baru yang dibuat oleh transaksi ini.

### 3.2. Block

Sebuah block terdiri dari header dan body.

```json
{
  "header": {
    "version": "number",
    "prevHash": "string (hash dari header block sebelumnya)",
    "merkleRoot": "string (hash dari root Merkle Tree transaksi)",
    "timestamp": "number (Unix timestamp)",
    "difficulty": "number (target kesulitan saat ini)",
    "nonce": "number (solusi dari PoW)"
  },
  "body": {
    "transactions": [
      // Daftar objek transaksi
    ]
  }
}
```

- `prevHash`: Menghubungkan block ini ke block sebelumnya, membentuk rantai.
- `merkleRoot`: Memastikan integritas daftar transaksi tanpa perlu menyimpan seluruh data transaksi di header.

## 4. Aturan Konsensus: Proof of Work (PoW)

Swatantra menggunakan PoW untuk mencapai konsensus.

- **Mekanisme**: Penambang (miner) harus menemukan `nonce` sehingga hash Keccak256 dari `header` block menghasilkan nilai yang lebih kecil dari target kesulitan (`difficulty`).
  
  `hash(header) < target`

- **Penyesuaian Kesulitan (Difficulty Adjustment)**: Kesulitan disesuaikan setiap block menggunakan **Exponential Moving Average (EMA)** dari waktu block sebelumnya.
    - **Tujuan**: Menjaga waktu rata-rata antar block (block time) tetap stabil.
    - **Formula**: `new_difficulty = old_difficulty * (1 - alpha) + (actual_block_time / target_block_time) * alpha` (konsep disederhanakan).

- **Fork Choice Rule**: Jika terjadi fork, chain yang valid adalah yang memiliki **total kesulitan kumulatif (cumulative work) terbesar**.

## 5. Aturan Anti-Spam dan Jaringan

Untuk menjaga kesehatan jaringan, beberapa batasan diberlakukan.

- **Ukuran Block**: Ukuran maksimum per block dibatasi (misalnya, 1 MB) untuk mencegah block yang terlalu besar membebani jaringan.
- **Ukuran Mempool**: Setiap node akan membatasi jumlah transaksi yang disimpan di mempool untuk mencegah kehabisan memori.
- **Rate Limit Transaksi**: Node dapat memberlakukan batasan jumlah transaksi yang diterima dari satu peer dalam periode waktu tertentu untuk mencegah serangan spam.
