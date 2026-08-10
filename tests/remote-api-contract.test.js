import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import evidence from '../spec/partiful.api-evidence.json';
import spec from '../spec/partiful.openapi.json';

const methods = new Set(['get', 'post', 'put', 'patch', 'delete']);
const ignoredKeys = new Set(['description', 'summary', 'title']);
const materialMapKeys = new Set([
  'properties',
  'responses',
  'content',
  'schemas',
  'parameters',
  'requestBodies',
  'securitySchemes',
]);
const hosts = {
  firebaseCallable: 'https://api.partiful.com',
  firestore: 'https://firestore.googleapis.com',
  token: 'https://securetoken.googleapis.com',
  identity: 'https://identitytoolkit.googleapis.com',
  upload: 'https://us-central1-getpartiful.cloudfunctions.net',
  posters: 'https://assets.getpartiful.com',
};

function operations() {
  return Object.entries(spec.paths).flatMap(([path, item]) =>
    Object.entries(item)
      .filter(([method]) => methods.has(method))
      .map(([method, operation]) => ({ path, method, operation })),
  );
}

function escapePointerSegment(value) {
  return value.replaceAll('~', '~0').replaceAll('/', '~1');
}

function materialClaimPointers(value, pointer, parentKey = '') {
  if (value === null || typeof value !== 'object') {
    return ignoredKeys.has(parentKey) ? [] : [pointer];
  }
  if (Array.isArray(value)) {
    return value.flatMap((item, index) =>
      materialClaimPointers(item, `${pointer}/${index}`, parentKey),
    );
  }
  return Object.entries(value).flatMap(([key, child]) => {
    const childPointer = `${pointer}/${escapePointerSegment(key)}`;
    const namedClaim = materialMapKeys.has(parentKey) ? [childPointer] : [];
    return [
      ...namedClaim,
      ...(ignoredKeys.has(key) ? [] : materialClaimPointers(child, childPointer, key)),
    ];
  });
}

function markdownSlug(value) {
  return value
    .toLowerCase()
    .replace(/[`*_]/g, '')
    .replace(/[^a-z0-9 -]/g, '')
    .trim()
    .replace(/\s+/g, '-');
}

function citationResolves(citation) {
  const separator = citation.indexOf('#');
  if (separator < 1) return false;
  const sourcePath = citation.slice(0, separator);
  const fragment = citation.slice(separator + 1);
  if (path.isAbsolute(sourcePath) || sourcePath.split('/').includes('..')) return false;
  const source = path.resolve(process.cwd(), sourcePath);
  if (!fs.existsSync(source)) return false;

  if (sourcePath.endsWith('.json')) {
    let value;
    try {
      value = JSON.parse(fs.readFileSync(source, 'utf8'));
    } catch {
      return false;
    }
    for (const rawSegment of fragment.replace(/^\//, '').split('/')) {
      if (!rawSegment) continue;
      const segment = rawSegment.replaceAll('~1', '/').replaceAll('~0', '~');
      if (value === null || typeof value !== 'object' || !(segment in value)) return false;
      value = value[segment];
    }
    return true;
  }

  if (sourcePath.endsWith('.md')) {
    const anchors = [...fs.readFileSync(source, 'utf8').matchAll(/^#{1,6}\s+(.+)$/gm)]
      .map((match) => markdownSlug(match[1]));
    return anchors.includes(fragment);
  }

  return false;
}

describe('remote API contract', () => {
  it('is a proposed, consistently versioned OpenAPI 3.1 document with unique operation IDs', () => {
    expect(spec.openapi).toBe('3.1.0');
    expect(spec.info.version).toBe(evidence.contractRevision);
    expect(evidence.status).toBe('proposed-pending-owner-approval');
    expect(spec.info.description).toContain('Proposed');
    const ids = operations().map(({ operation }) => operation.operationId);
    expect(ids).toHaveLength(27);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('covers all 379 material OpenAPI claims with a resolving citation', () => {
    const allowed = new Set(evidence.allowedClassifications);
    for (const { operation } of operations()) {
      const operationEvidence = evidence.operations[operation.operationId];
      expect(operationEvidence, operation.operationId).toBeDefined();
      expect(allowed.has(operationEvidence.classification)).toBe(true);
      expect(citationResolves(operationEvidence.citation), operation.operationId).toBe(true);
      const claim = evidence.operationClaims[operation.operationId];
      expect(claim, operation.operationId).toBeDefined();
      expect(allowed.has(claim.request)).toBe(true);
      expect(allowed.has(claim.response)).toBe(true);
      expect(claim.status).toBe('explicit-unknown');
      expect(citationResolves(claim.citation), operation.operationId).toBe(true);
      expect(citationResolves(claim.statusCitation), operation.operationId).toBe(true);
    }
    const pointers = new Set([
      ...materialClaimPointers(spec.servers, '#/servers', 'servers'),
      ...materialClaimPointers(spec.paths, '#/paths', 'paths'),
      ...materialClaimPointers(spec.components, '#/components', 'components'),
    ]);
    expect(pointers).toHaveLength(379);
    for (const pointer of pointers) {
      const claim = evidence.claims[pointer];
      expect(claim, pointer).toBeDefined();
      expect(allowed.has(claim.classification), pointer).toBe(true);
      expect(citationResolves(claim.citation), pointer).toBe(true);
    }
  });

  it('contains remote transport facts rather than product or implementation policy', () => {
    const serialized = JSON.stringify(spec).toLowerCase();
    for (const forbidden of ['x-mcp', 'x-pp-', 'printing press', 'credential', 'environment variable', 'workflow', 'command layer']) {
      expect(serialized).not.toContain(forbidden);
    }
  });

  it('keeps operation servers consistent with their transport paths', () => {
    for (const { path, operation } of operations()) {
      const server = operation.servers?.[0]?.url ?? spec.servers[0].url;
      if (path.startsWith('/v1/projects/')) expect(server).toBe(hosts.firestore);
      else if (path === '/v1/token') expect(server).toBe(hosts.token);
      else if (path.startsWith('/v1/accounts:')) expect(server).toBe(hosts.identity);
      else if (path === '/uploadPhoto') expect(server).toBe(hosts.upload);
      else if (path === '/posters.json') expect(server).toBe(hosts.posters);
      else expect(server).toBe(hosts.firebaseCallable);
    }
  });

  it('does not assert an unknown status code as a success response', () => {
    for (const { operation } of operations()) {
      expect(Object.keys(operation.responses)).toEqual(['default']);
      expect(evidence.operationClaims[operation.operationId].status).toBe('explicit-unknown');
    }
  });

  it('uses the observed nested text-blast message and excludes the superseded shape', () => {
    const params = spec.components.schemas.TextBlastRequest.properties.data.properties.params;
    expect(params.properties.message.$ref).toBe('#/components/schemas/TextBlastMessage');
    expect(params.properties).not.toHaveProperty('recipientStatuses');
    expect(spec.components.schemas.TextBlastMessage.required).toEqual(
      expect.arrayContaining(['text', 'to', 'showOnEventPage']),
    );
    expect(evidence.contradictions.find(({ id }) => id === 'text-blast-message-shape')?.status).toBe('resolved');
  });

  it('does not treat the event-image observation as poster-catalog evidence', () => {
    const posterOperation = evidence.operations.getPosterCatalog;
    expect(posterOperation.classification).toBe('typescript-derived-inference');
    expect(posterOperation.citation).toBe(evidence.sources.posterCatalog);
    expect(evidence.sources).not.toHaveProperty('eventImageObservation');
    for (const [pointer, claim] of Object.entries(evidence.claims)) {
      if (pointer.includes('/posters.json') || pointer.includes('/Poster')) {
        expect(claim.citation).not.toBe(evidence.sources.eventImageObservation);
      }
    }
  });
});
