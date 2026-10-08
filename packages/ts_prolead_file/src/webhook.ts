import { createHmac, timingSafeEqual } from 'node:crypto';

/**
 * Validates HMAC-SHA256 signature from Prolead File webhooks.
 *
 * @param payload Raw payload string or Buffer
 * @param headerSignature Value from `X-Prolead-Signature` header (e.g. `sha256=...` or raw hex)
 * @param secret Webhook endpoint signing secret
 */
export function verifyWebhookSignature(
  payload: string | Buffer | Uint8Array,
  headerSignature: string,
  secret: string
): boolean {
  if (!secret || !headerSignature) return false;

  const cleanSig = headerSignature.replace(/^sha256=/, '').trim();
  const rawBytes = typeof payload === 'string' ? Buffer.from(payload, 'utf-8') : Buffer.from(payload);

  const hmac = createHmac('sha256', secret);
  hmac.update(rawBytes);
  const digest = hmac.digest('hex');

  try {
    const sigBuf = Buffer.from(cleanSig, 'hex');
    const digestBuf = Buffer.from(digest, 'hex');
    if (sigBuf.length !== digestBuf.length) return false;
    return timingSafeEqual(sigBuf, digestBuf);
  } catch (_) {
    return false;
  }
}
