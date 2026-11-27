import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import ProvidersPage from './+page.svelte';
import { garmApi } from '$lib/api/client.js';

vi.mock('$app/stores', () => ({}));

vi.mock('$app/navigation', () => ({}));

vi.unmock('$lib/components/PageHeader.svelte');

vi.mock('$lib/utils/jwt', () => ({
	isCurrentUserAdmin: () => true
}));

vi.mock('$lib/stores/toast.js', () => ({
	toastStore: {
		success: vi.fn(),
		error: vi.fn(),
		info: vi.fn()
	}
}));

vi.mock('$lib/api/client.js', () => ({
	garmApi: {
		listProviders: vi.fn(),
		getProviderRunnerConfig: vi.fn(),
		updateProviderRunnerConfig: vi.fn()
	}
}));

const schedulerProvider = { id: 1, name: 'vscheduler', type: 'external' };
const lxdProvider = { id: 2, name: 'lxd', type: 'lxd' };
const schedulerConfig = {
	supported: true,
	base_url: 'http://v-scheduler:38120/api/v1',
	image_repository: 'registry.example.com/ci/runner',
	image_tag: 'v6.0.1_27ec9940',
	token_configured: true
};

describe('Providers page', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(garmApi.listProviders).mockResolvedValue([schedulerProvider]);
		vi.mocked(garmApi.getProviderRunnerConfig).mockResolvedValue(schedulerConfig);
	});

	it('keeps the form and shows the error when saving fails', async () => {
		vi.mocked(garmApi.updateProviderRunnerConfig).mockRejectedValue(
			new Error('base_url must be an absolute http or https URL')
		);
		render(ProvidersPage);

		const save = await screen.findByRole('button', { name: 'Save' });
		await fireEvent.click(save);

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('base_url must be an absolute http or https URL');
		expect(screen.getByRole('textbox', { name: /scheduler base url/i })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument();
	});

	it('saves the form values without an alert', async () => {
		vi.mocked(garmApi.updateProviderRunnerConfig).mockResolvedValue(schedulerConfig);
		render(ProvidersPage);

		const save = await screen.findByRole('button', { name: 'Save' });
		await fireEvent.click(save);

		await waitFor(() => expect(garmApi.updateProviderRunnerConfig).toHaveBeenCalledTimes(1));
		expect(garmApi.updateProviderRunnerConfig).toHaveBeenCalledWith('vscheduler', {
			base_url: 'http://v-scheduler:38120/api/v1',
			token: '',
			image_repository: 'registry.example.com/ci/runner',
			image_tag: 'v6.0.1_27ec9940'
		});
		expect(screen.queryByRole('alert')).not.toBeInTheDocument();
	});

	it('shows the load error when the config cannot be read', async () => {
		vi.mocked(garmApi.getProviderRunnerConfig).mockRejectedValue(new Error('only administrators'));
		render(ProvidersPage);

		const alert = await screen.findByRole('alert');
		expect(alert.textContent).toContain('only administrators');
		expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
	});

	it('explains providers without runner settings', async () => {
		vi.mocked(garmApi.listProviders).mockResolvedValue([lxdProvider]);
		render(ProvidersPage);

		expect(await screen.findByText(/does not use garm runner settings/i)).toBeInTheDocument();
		expect(garmApi.getProviderRunnerConfig).not.toHaveBeenCalled();
	});
});
