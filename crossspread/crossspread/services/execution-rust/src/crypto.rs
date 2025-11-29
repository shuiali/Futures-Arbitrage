//! Cryptographic utilities for API key encryption/decryption

use aes_gcm::{
    aead::{Aead, KeyInit},
    Aes256Gcm, Nonce,
};
use anyhow::{Context, Result};
use rand::Rng;

const NONCE_SIZE: usize = 12;

/// Encrypt plaintext using AES-256-GCM
pub fn encrypt(key: &[u8], plaintext: &[u8]) -> Result<Vec<u8>> {
    if key.len() != 32 {
        anyhow::bail!("Encryption key must be 32 bytes");
    }

    let cipher = Aes256Gcm::new_from_slice(key)
        .context("Failed to create cipher")?;

    // Generate random nonce
    let mut nonce_bytes = [0u8; NONCE_SIZE];
    rand::thread_rng().fill(&mut nonce_bytes);
    let nonce = Nonce::from_slice(&nonce_bytes);

    // Encrypt
    let ciphertext = cipher
        .encrypt(nonce, plaintext)
        .map_err(|e| anyhow::anyhow!("Encryption failed: {}", e))?;

    // Prepend nonce to ciphertext
    let mut result = Vec::with_capacity(NONCE_SIZE + ciphertext.len());
    result.extend_from_slice(&nonce_bytes);
    result.extend_from_slice(&ciphertext);

    Ok(result)
}

/// Decrypt ciphertext using AES-256-GCM
pub fn decrypt(key: &[u8], ciphertext: &[u8]) -> Result<Vec<u8>> {
    if key.len() != 32 {
        anyhow::bail!("Encryption key must be 32 bytes");
    }

    if ciphertext.len() < NONCE_SIZE {
        anyhow::bail!("Ciphertext too short");
    }

    let cipher = Aes256Gcm::new_from_slice(key)
        .context("Failed to create cipher")?;

    // Extract nonce and ciphertext
    let nonce = Nonce::from_slice(&ciphertext[..NONCE_SIZE]);
    let encrypted = &ciphertext[NONCE_SIZE..];

    // Decrypt
    let plaintext = cipher
        .decrypt(nonce, encrypted)
        .map_err(|e| anyhow::anyhow!("Decryption failed: {}", e))?;

    Ok(plaintext)
}

/// Decrypt API credentials from database
pub fn decrypt_credentials(
    key: &[u8],
    api_key_encrypted: &[u8],
    api_secret_encrypted: &[u8],
    passphrase_encrypted: Option<&[u8]>,
) -> Result<(String, String, Option<String>)> {
    let api_key = String::from_utf8(decrypt(key, api_key_encrypted)?)
        .context("API key is not valid UTF-8")?;
    
    let api_secret = String::from_utf8(decrypt(key, api_secret_encrypted)?)
        .context("API secret is not valid UTF-8")?;
    
    let passphrase = if let Some(encrypted) = passphrase_encrypted {
        Some(String::from_utf8(decrypt(key, encrypted)?)
            .context("Passphrase is not valid UTF-8")?)
    } else {
        None
    };

    Ok((api_key, api_secret, passphrase))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_encrypt_decrypt() {
        let key = [0u8; 32]; // Test key
        let plaintext = b"my_secret_api_key";

        let encrypted = encrypt(&key, plaintext).unwrap();
        let decrypted = decrypt(&key, &encrypted).unwrap();

        assert_eq!(plaintext.to_vec(), decrypted);
    }
}
