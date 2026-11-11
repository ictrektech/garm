import { browser } from '$app/environment';
import { APP_BASE_PATH } from './base-path';

interface VOSOAuth2 {
	authorize(params: Record<string, unknown>): Promise<{ code: string; state: string }>;
	token(params: Record<string, unknown>): Promise<{ access_token?: string }>;
}

declare global {
	interface Window {
		vos_platform?: { api?: { v1000?: { oauth2?: VOSOAuth2 } } };
	}
}

const CLIENT_ID = 'com.ictrek.garm';
const SCOPE = 'openid profile email';
const BRIDGE_TIMEOUT_MS = 3000;

function randomBase64Url(length = 32): string {
	const bytes = crypto.getRandomValues(new Uint8Array(length));
	let binary = '';
	for (const byte of bytes) binary += String.fromCharCode(byte);
	return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

async function oauth2Bridge(): Promise<VOSOAuth2 | null> {
	if (!browser) return null;
	const current = window.vos_platform?.api?.v1000?.oauth2;
	if (current) return current;
	if (window.parent === window) return null;

	const started = Date.now();
	while (Date.now() - started < BRIDGE_TIMEOUT_MS) {
		await new Promise((resolve) => window.setTimeout(resolve, 50));
		const bridge = window.vos_platform?.api?.v1000?.oauth2;
		if (bridge) return bridge;
	}
	return null;
}

/** Runs the VOS OIDC PKCE flow. Returns null outside the VOS shell. */
async function requestVOSAccessToken(): Promise<string | null> {
	const bridge = await oauth2Bridge();
	if (!bridge) return null;

	const verifier = randomBase64Url();
	const state = randomBase64Url();
	const nonce = randomBase64Url();
	const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier)));

	const authorized = await bridge.authorize({
		client_id: CLIENT_ID,
		response_type: 'code',
		scope: SCOPE,
		state,
		nonce,
		code_challenge: randomBase64UrlFromBytes(digest),
		code_challenge_method: 'S256'
	});
	if (!authorized?.code || authorized.state !== state) return null;

	const tokens = await bridge.token({
		grant_type: 'authorization_code',
		code: authorized.code,
		code_verifier: verifier,
		client_id: CLIENT_ID
	});
	return tokens?.access_token ?? null;
}

function randomBase64UrlFromBytes(bytes: Uint8Array): string {
	let binary = '';
	for (const byte of bytes) binary += String.fromCharCode(byte);
	return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/**
 * Exchanges a VOS access token for a GARM JWT.
 * Returns null when GARM rejects the token.
 */
export async function tryVOSLogin(): Promise<{ token: string; username: string } | null> {
	if (!browser) return null;
	try {
		const accessToken = await requestVOSAccessToken();
		if (!accessToken) return null;

		const response = await fetch(`${APP_BASE_PATH}/api/v1/auth/vos`, {
			method: 'POST',
			credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ access_token: accessToken })
		});
		if (!response.ok) return null;

		const payload = (await response.json()) as { token?: string; username?: string };
		return payload.token && payload.username ? { token: payload.token, username: payload.username } : null;
	} catch {
		return null;
	}
}
