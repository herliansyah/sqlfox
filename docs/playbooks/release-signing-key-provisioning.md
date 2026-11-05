# Release Signing Key Provisioning Guide

Panduan operasional untuk memprovisikan dan merotasi kunci penandatangan manifest rilis SqlFox (Ed25519). Kunci ini adalah **trust anchor rantai pasok** untuk fitur in-app self-update: binary yang dirilis hanya akan mempercayai `checksums.txt` jika disertai tanda tangan (`checksums.txt.sig`) yang valid terhadap public key yang tertanam di dalamnya.

Tanpa provisioning ini, updater tetap aman dengan perilaku lama (verifikasi checksum same-origin), tetapi belum mendapat proteksi tanda tangan yang melindungi dari rilis GitHub yang dikompromikan.

> Latar belakang keputusan arsitektur: lihat `docs/adr/0060-supply-chain-signing-and-ingress-crypto-hardening.md` dan `SECURITY.md` bagian 8.

---

## 1. Kapan Menjalankan Ini

- **Sekali saat setup awal** sebelum rilis pertama yang ingin ditandatangani.
- **Saat rotasi kunci**: jika private key diduga bocor, atau sebagai praktik rotasi berkala.
- Anda **tidak** perlu menjalankannya setiap rilis — penandatanganan per-rilis berjalan otomatis di GitHub Actions setelah kunci terpasang.

---

## 2. Membuat Keypair

Jalankan di lingkungan tepercaya (mesin maintainer, bukan CI publik, bukan shell yang ter-log):

```bash
go run scripts/sign-manifest.go -genkey
```

Output (contoh — JANGAN pakai nilai ini, selalu generate sendiri):

```
PUBLIC_KEY=2J8FDYfIALEjv/2HvCiCD+OvFmoYrhi5bBH8BLSkZZA=
PRIVATE_KEY=aJQNXNH4f/F+/JSgjIDLDTT9WSZdMTpCPlUqsnYa4l/YnwUNh8g...
```

- `PUBLIC_KEY` — **tidak rahasia**. Ditanamkan ke binary saat build.
- `PRIVATE_KEY` — **rahasia**. Hanya menandatangani manifest; simpan offline.

> ⚠️ Helper ini CGO-free dan hanya memakai `crypto/ed25519` dari standar library (menghormati CGO-Free Invariant). Signer `scripts/sign-manifest.go` memakai build tag `ignore` sehingga tidak pernah ikut ter-compile ke dalam binary yang dirilis.

---

## 3. Memasang Kunci di GitHub

Repo → **Settings** → **Secrets and variables** → **Actions**:

1. Tab **Secrets** → **New repository secret**
   - Name: `SQLFOX_MANIFEST_SIGNING_KEY`
   - Value: isi `PRIVATE_KEY` (base64 penuh, termasuk `==` di akhir jika ada).

2. Tab **Variables** → **New repository variable**
   - Name: `SQLFOX_MANIFEST_SIGNING_PUBKEY`
   - Value: isi `PUBLIC_KEY`.

3. **Simpan `PRIVATE_KEY` secara offline** (password manager / vault organisasi) sebagai cadangan.

Workflow rilis (`.github/workflows/release.yml`) sudah membaca keduanya:
- `PUBLIC_KEY` ditanamkan via `-ldflags "-X sqlfox/internal/server.manifestSigningPublicKeyB64=<pubkey>"`.
- `PRIVATE_KEY` dipakai step "Sign Checksum Manifest" untuk menghasilkan `checksums.txt.sig`.

Step penandatanganan **fail-closed**: jika `SQLFOX_MANIFEST_SIGNING_PUBKEY` disetel tetapi secret `SQLFOX_MANIFEST_SIGNING_KEY` hilang, rilis dibatalkan (bukan merilis binary yang mewajibkan tanda tangan tanpa menyertakan tanda tangannya).

---

## 4. Memverifikasi Secara Lokal (opsional, disarankan sebelum rilis pertama)

Buktikan keypair bekerja end-to-end sebelum mengandalkannya:

```bash
# Tandatangani manifest contoh dengan private key Anda
printf 'deadbeef  sqlfox-vX.Y.Z-linux-amd64.tar.gz\n' > /tmp/checksums.txt
go run scripts/sign-manifest.go -sign -key "$PRIVATE_KEY" /tmp/checksums.txt
# -> menulis /tmp/checksums.txt.sig
```

Kontrak penandatangan ↔ verifikasi dijamin oleh unit test `TestVerifyAssetChecksum_Ed25519ManifestSignature`
(`internal/server/update_test.go`): tanda tangan valid diterima, tanda tangan hilang/salah ditolak (fail-closed).

Untuk menguji binary ber-key langsung:

```bash
go build -ldflags "-X sqlfox/internal/server.manifestSigningPublicKeyB64=$PUBLIC_KEY" -o /tmp/sqlfox ./cmd/sqlfox
/tmp/sqlfox -version
```

---

## 5. Alur Rilis Setelah Kunci Terpasang

Tidak ada langkah manual tambahan. Jalankan rilis seperti biasa:

```bash
./scripts/release.sh <patch|minor|major|vX.Y.Z> --push
```

GitHub Actions akan:
1. Build semua platform dengan public key tertanam.
2. Membuat `checksums.txt` dari arsip.
3. Menandatanganinya menjadi `checksums.txt.sig`.
4. Mengunggah keduanya sebagai aset rilis.

Binary hasil rilis kemudian **mewajibkan** `checksums.txt.sig` yang valid sebelum mempercayai checksum apa pun saat update.

---

## 6. Rotasi & Pemulihan Kunci

- **Rotasi**: generate keypair baru (bagian 2), perbarui secret + variable (bagian 3). Rilis baru memakai kunci baru; rilis lama tetap terverifikasi oleh binary lama (yang menanam kunci lama).
- **Kehilangan private key**: cukup generate keypair baru dan perbarui kedua setting. Tidak ada kerusakan pada rilis yang sudah ada — hanya rilis mendatang yang memerlukan kunci baru.
- **Dugaan kebocoran private key**: rotasi segera. Karena tiap binary menanam public key yang berlaku pada saat build-nya, binary lama tidak terpengaruh kunci yang bocor selama Anda tidak menandatangani rilis jahat dengannya — tetap rotasi untuk mencegah penyalahgunaan di masa depan.

---

## 7. Daftar Periksa Cepat

- [ ] `go run scripts/sign-manifest.go -genkey` dijalankan di lingkungan tepercaya.
- [ ] `SQLFOX_MANIFEST_SIGNING_KEY` (secret) diisi `PRIVATE_KEY`.
- [ ] `SQLFOX_MANIFEST_SIGNING_PUBKEY` (variable) diisi `PUBLIC_KEY`.
- [ ] `PRIVATE_KEY` dicadangkan offline.
- [ ] Verifikasi lokal lolos (bagian 4) — opsional tapi disarankan.
- [ ] Rilis percobaan menghasilkan `checksums.txt.sig` sebagai aset.
