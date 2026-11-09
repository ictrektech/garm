declare global {
	namespace App {
		interface Error {}
		interface Locals {}
		interface PageData {}
		interface Platform {}
	}

	interface ImportMetaEnv {
		readonly VITE_GARM_BASE_PATH?: string;
	}
}

export {};