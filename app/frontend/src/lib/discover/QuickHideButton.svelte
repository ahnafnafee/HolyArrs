<script lang="ts">
	import { api, message } from '#lib/core/api.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { quickHide, showAgain } from './hidden.ts';

	let { kind, tmdb, title, onhidden }: {
		kind: 'movie' | 'show';
		tmdb: string;
		title: string;
		onhidden: () => void;
	} = $props();

	let busy = $state(false);

	async function hide() {
		const userId = session.s.id;
		if (!userId || busy) return;
		busy = true;
		try {
			const hidden = await quickHide(api, userId, kind, tmdb, title);
			onhidden();
			toasts.push(t('discover.hidden.hiddenForYou', { name: title }), 'ok', {
				action: {
					label: t('discover.hidden.undo'),
					run: () => showAgain(api, hidden.id).catch((err) => toasts.push(message(err), 'fail'))
				}
			});
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busy = false;
		}
	}
</script>

{#if session.can('users') && session.s.id && /^[1-9]\d*$/.test(tmdb)}
	<button type="button" class="quick-hide" aria-label={t('discover.hidden.quick', { name: title })} title={t('discover.hidden.quickTip')} disabled={busy} onclick={hide}>
		<Icon name="eyeOff" size={18} />
	</button>
{/if}

<style>
	.quick-hide { flex: none; width: 36px; height: 36px; display: grid; place-items: center; border-radius: 50%; color: var(--text); box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--text) 40%, transparent); }
	.quick-hide:hover { box-shadow: inset 0 0 0 2px var(--text); background: var(--surface-2); }
	.quick-hide:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.quick-hide:disabled { opacity: var(--disabled-opacity); }
</style>
