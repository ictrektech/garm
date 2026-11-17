import { writable, get } from 'svelte/store';

/**
 * Theme mode. `system` follows the OS preference and keeps following it;
 * `light` / `dark` pin an explicit choice.
 * @typedef {'system' | 'light' | 'dark'} ThemeMode
 */

const STORAGE_KEY = 'theme';
/** @type {ThemeMode[]} */
const MODES = ['system', 'light', 'dark'];

function prefersDark() {
	return typeof window !== 'undefined' &&
		window.matchMedia('(prefers-color-scheme: dark)').matches;
}

/** @returns {ThemeMode} */
function readSavedMode() {
	const saved = localStorage.getItem(STORAGE_KEY);
	return saved === 'light' || saved === 'dark' || saved === 'system' ? saved : 'system';
}

/** @param {ThemeMode} mode */
function resolveDark(mode) {
	return mode === 'system' ? prefersDark() : mode === 'dark';
}

/**
 * @param {MediaQueryList | null} mediaQuery
 * @param {() => void} handler
 */
function addMediaListener(mediaQuery, handler) {
	if (!mediaQuery) return;
	if (typeof mediaQuery.addEventListener === 'function') {
		mediaQuery.addEventListener('change', handler);
	} else if (typeof mediaQuery.addListener === 'function') {
		mediaQuery.addListener(handler);
	}
}

/**
 * @param {MediaQueryList | null} mediaQuery
 * @param {() => void} handler
 */
function removeMediaListener(mediaQuery, handler) {
	if (!mediaQuery) return;
	if (typeof mediaQuery.removeEventListener === 'function') {
		mediaQuery.removeEventListener('change', handler);
	} else if (typeof mediaQuery.removeListener === 'function') {
		mediaQuery.removeListener(handler);
	}
}

/** @param {boolean} isDark */
function applyDocumentClass(isDark) {
	document.documentElement.classList.toggle('dark', isDark);

	const fallbackIcon = document.getElementById('favicon-fallback');
	const href = fallbackIcon?.getAttribute('href');
	if (fallbackIcon && href) {
		const icon = isDark ? 'favicon-dark.png' : 'favicon-light.png';
		fallbackIcon.setAttribute('href', href.replace(/favicon-(light|dark)\.png/, icon));
	}
}

function createThemeStore() {
	const state = writable({ mode: /** @type {ThemeMode} */ ('system'), isDark: false });

	/** @type {MediaQueryList | null} */
	let mediaQuery = null;

	/**
	 * @param {ThemeMode} mode
	 * @param {boolean} persist write the choice to localStorage
	 */
	function publish(mode, persist) {
		if (persist) {
			localStorage.setItem(STORAGE_KEY, mode);
		}
		const isDark = resolveDark(mode);
		applyDocumentClass(isDark);
		state.set({ mode, isDark });
	}

	function handleSystemChange() {
		// Follow the OS only while the user has not pinned a theme.
		if (get(state).mode === 'system') {
			publish('system', false);
		}
	}

	/** Re-bind the listener so repeated init() calls never double-subscribe. */
	function attachMediaListener() {
		if (mediaQuery) {
			removeMediaListener(mediaQuery, handleSystemChange);
			mediaQuery = null;
		}
		mediaQuery = window.matchMedia?.('(prefers-color-scheme: dark)') ?? null;
		addMediaListener(mediaQuery, handleSystemChange);
	}

	return {
		subscribe: state.subscribe,

		/** Apply the saved mode and start following the OS preference. */
		init() {
			publish(readSavedMode(), false);
			attachMediaListener();
		},

		/** Stop following the OS preference. */
		destroy() {
			if (!mediaQuery) return;
			removeMediaListener(mediaQuery, handleSystemChange);
			mediaQuery = null;
		},

		/** @param {ThemeMode} mode */
		setMode(mode) {
			publish(mode, true);
		},

		/** Cycle system → light → dark → system. */
		toggle() {
			const current = get(state).mode;
			const next = MODES[(MODES.indexOf(current) + 1) % MODES.length];
			publish(next, true);
		}
	};
}

export const themeStore = createThemeStore();
