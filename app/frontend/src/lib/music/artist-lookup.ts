import { api } from '#lib/core/api.ts';
import { askAgainOnce } from './music-pages.ts';
import { reaskWhileFilling } from './reask.ts';
import type { LookupArtist } from './types.ts';

export const variousArtistsMbid = '89ad4ac3-39f7-470e-963a-56509c546377';

export async function lookupArtist(mbid: string, signal: AbortSignal): Promise<LookupArtist | null> {
	const path = `/artists/lookup?term=${encodeURIComponent(`mbid:${mbid}`)}`;
	const ask = () => {
		signal.throwIfAborted();
		return api.get<{ artists?: LookupArtist[]; pending?: boolean; partial?: boolean }>(path, signal);
	};
	const first = await askAgainOnce(ask);
	return (await reaskWhileFilling(first, ask, () => {}, signal)).artists?.[0] ?? null;
}
