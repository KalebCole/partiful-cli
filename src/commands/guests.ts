/**
 * Guests commands: list, invite
 */

import type { Command } from 'commander';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest, firestoreListDocuments } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

interface GuestRecord {
  name: string;
  status: string;
  createdAt: string | null;
  inviteDate: string | null;
  count: number;
  channel: string | null;
}

interface FirestoreDoc {
  fields?: Record<string, {
    stringValue?: string;
    timestampValue?: string;
    integerValue?: string;
    mapValue?: { fields?: Record<string, { stringValue?: string }> };
  }>;
}

export async function fetchGuests(eventId: string, token: string, config: ReturnType<typeof loadConfig>, verbose = false): Promise<GuestRecord[]> {
  const guests: GuestRecord[] = [];
  let pageToken: string | null = null;
  do {
    const result = await firestoreListDocuments(
      `events/${eventId}/guests`, token, 100, pageToken, verbose
    ) as { documents?: FirestoreDoc[]; nextPageToken?: string };
    if (result.documents) {
      for (const doc of result.documents) {
        const f = doc.fields ?? {};
        guests.push({
          name: f['name']?.stringValue ?? 'Unknown',
          status: f['status']?.stringValue ?? 'UNKNOWN',
          createdAt: f['createdAt']?.timestampValue ?? null,
          inviteDate: f['inviteDate']?.timestampValue ?? null,
          count: parseInt(f['count']?.integerValue ?? '1'),
          channel: f['inviteMetadata']?.mapValue?.fields?.['channel']?.stringValue ?? null,
        });
      }
    }
    pageToken = result.nextPageToken ?? null;
  } while (pageToken);
  return guests;
}

export function registerGuestsCommands(program: Command): void {
  const guests = program.command('guests').description('Manage event guests');

  guests
    .command('list')
    .description('List guests for an event')
    .argument('<eventId>', 'Event ID')
    .option('--status <status>', 'Filter by RSVP status (GOING, MAYBE, SENT, DECLINED, WAITLIST)')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Fetch event metadata for counts
        const eventPayload = {
          data: wrapPayload(config, {
            params: { eventId },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        let counts: Record<string, number> = {};
        let eventTitle = 'Unknown Event';
        try {
          const eventResult = await apiRequest('POST', '/getEventInfo', token, eventPayload, globalOpts['verbose'] as boolean | undefined) as { result?: { data?: { event?: { title?: string; guestStatusCounts?: Record<string, number> } } } };
          const event = eventResult.result?.data?.event;
          if (event) {
            eventTitle = event.title ?? 'Unknown Event';
            counts = event.guestStatusCounts ?? {};
          }
        } catch {
          // API may be down, continue with Firestore guest fetch
        }

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, eventId, collection: `events/${eventId}/guests` });
          return;
        }

        let guestList = await fetchGuests(eventId, token, config, globalOpts['verbose'] as boolean | undefined);

        // Compute counts from guest list if API didn't provide them
        if (Object.keys(counts).length === 0 && guestList.length > 0) {
          for (const g of guestList) {
            counts[g.status] = (counts[g.status] ?? 0) + 1;
          }
        }

        // Filter by status
        if (opts['status']) {
          const statusFilter = (opts['status'] as string).toUpperCase();
          guestList = guestList.filter(g => g.status === statusFilter);
        }

        jsonOutput({
          eventId,
          eventTitle,
          guests: guestList,
          counts: {
            going: counts['GOING'] ?? 0,
            maybe: counts['MAYBE'] ?? 0,
            invited: counts['SENT'] ?? 0,
            declined: counts['DECLINED'] ?? 0,
            waitlist: counts['WAITLIST'] ?? 0,
          },
          total: guestList.length,
        });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError((e as Error).message);
      }
    });

  guests
    .command('invite')
    .description('Send invites to an event')
    .argument('<eventId>', 'Event ID')
    .option('--phone <phones...>', 'Phone number(s) to invite')
    .option('--user-id <userIds...>', 'Partiful user ID(s) to invite')
    .option('--message <msg>', 'Optional invitation message')
    .action(async (eventId: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const userIdsToInvite = (opts['userId'] as string[] | undefined) ?? [];
        const phoneContactsToInvite = ((opts['phone'] as string[] | undefined) ?? []).map((phone: string) => ({
          phoneNumber: phone.replace(/[^+\d]/g, ''),
          firstName: '',
          lastName: '',
        }));

        if (userIdsToInvite.length === 0 && phoneContactsToInvite.length === 0) {
          jsonError('Provide --phone or --user-id to invite', 3, 'validation_error');
          return;
        }

        const payload = {
          data: wrapPayload(config, {
            params: {
              eventId,
              userIdsToInvite,
              phoneContactsToInvite,
              invitationMessage: (opts['message'] as string | undefined) ?? '',
              otherMutualsCount: 0,
            },
            amplitudeSessionId: Date.now(),
            userId: config.userId,
          }),
        };

        if (globalOpts['dryRun']) {
          jsonOutput({ dryRun: true, endpoint: '/addInvitedGuestsAsHost', payload });
          return;
        }

        await apiRequest('POST', '/addInvitedGuestsAsHost', token, payload, globalOpts['verbose'] as boolean | undefined);

        const invited = userIdsToInvite.length + phoneContactsToInvite.length;
        jsonOutput({
          eventId,
          invited,
          url: `https://partiful.com/e/${eventId}`,
        });
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError((e as Error).message);
      }
    });
}
