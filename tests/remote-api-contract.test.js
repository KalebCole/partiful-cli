import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import evidence from '../spec/partiful.api-evidence.json';
import spec from '../spec/partiful.openapi.json';

const historicalDraftPath = 'spec/research/historical-27-operation-draft.json';
const historicalDraft = JSON.parse(fs.readFileSync(historicalDraftPath, 'utf8'));
const sourceCache = new Map([[historicalDraftPath, historicalDraft]]);
const methods = new Set(['get', 'post', 'put', 'patch', 'delete']);
const ignoredKeys = new Set(['description', 'summary', 'title']);
const materialMapKeys = new Set([
  'properties',
  'responses',
  'content',
  'security',
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
    if (value.length === 0) return [pointer];
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

function jsonPointerValue(value, pointer) {
  for (const rawSegment of pointer.replace(/^#/, '').replace(/^\//, '').split('/')) {
    if (!rawSegment) continue;
    const segment = rawSegment.replaceAll('~1', '/').replaceAll('~0', '~');
    if (value === null || typeof value !== 'object' || !(segment in value)) return undefined;
    value = value[segment];
  }
  return value;
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
    let value = sourceCache.get(sourcePath);
    if (!value) {
      try {
        value = JSON.parse(fs.readFileSync(source, 'utf8'));
        sourceCache.set(sourcePath, value);
      } catch {
        return false;
      }
    }
    return jsonPointerValue(value, fragment) !== undefined;
  }

  if (sourcePath.endsWith('.md')) {
    const anchors = [...fs.readFileSync(source, 'utf8').matchAll(/^#{1,6}\s+(.+)$/gm)]
      .map((match) => markdownSlug(match[1]));
    return anchors.includes(fragment);
  }

  return false;
}

describe('remote API contract', () => {
  it('is a consistently versioned OpenAPI 3.1 document with unique operation IDs', () => {
    expect(spec.openapi).toBe('3.1.0');
    expect(spec.info.version).toBe(evidence.contractRevision);
    expect(['owner-reviewed', 'proposed']).toContain(evidence.status);
    const ids = operations().map(({ operation }) => operation.operationId);
    expect(ids).toHaveLength(27);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('covers all material OpenAPI claims with resolving and semantically supporting citations', () => {
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
      expect(citationResolves(claim.requestCitation), operation.operationId).toBe(true);
      expect(citationResolves(claim.responseCitation), operation.operationId).toBe(true);
      const observedStatusOps = new Set([
        'getPosterCatalog',
        'sendAuthCodeTrusted',
        'getLoginToken',
        'signInWithCustomToken',
        'refreshToken',
        'lookupFirebaseUser',
      ]);
      expect(claim.status).toBe(
        observedStatusOps.has(operation.operationId)
          ? 'dated-live-observation'
          : 'explicit-unknown',
      );
      expect(citationResolves(claim.statusCitation), operation.operationId).toBe(true);
    }
    const pointers = new Set([
      ...materialClaimPointers(spec.servers, '#/servers', 'servers'),
      ...materialClaimPointers(spec.paths, '#/paths', 'paths'),
      ...materialClaimPointers(spec.components, '#/components', 'components'),
    ]);
    expect(pointers).toHaveLength(992);
    for (const pointer of pointers) {
      const claim = evidence.claims[pointer];
      expect(claim, pointer).toBeDefined();
      expect(allowed.has(claim.classification), pointer).toBe(true);
      expect(citationResolves(claim.citation), pointer).toBe(true);
      if (claim.citation.startsWith(`${historicalDraftPath}#`)) {
        const sourcePointer = claim.citation.slice(
          historicalDraftPath.length,
        );
        const sourceValue = jsonPointerValue(historicalDraft, sourcePointer);
        const canonicalValue = jsonPointerValue(spec, pointer);
        expect(sourceValue, `${pointer} source value`).not.toBeUndefined();
        expect(canonicalValue, `${pointer} canonical value`).not.toBeUndefined();
        expect(sourceValue, pointer).toEqual(canonicalValue);
      }
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
    const observedStatusOps = new Set([
      'getPosterCatalog',
      'sendAuthCodeTrusted',
      'getLoginToken',
      'signInWithCustomToken',
      'refreshToken',
      'lookupFirebaseUser',
    ]);
    const observedErrorOps = {
      getLoginToken: ['200', '403', 'default'],
      signInWithCustomToken: ['200', '400', 'default'],
      refreshToken: ['200', '400', 'default'],
      lookupFirebaseUser: ['200', '400', 'default'],
    };
    for (const { operation } of operations()) {
      if (observedStatusOps.has(operation.operationId)) {
        const expectedKeys = observedErrorOps[operation.operationId] ?? ['200', 'default'];
        expect(Object.keys(operation.responses).sort()).toEqual(expectedKeys.sort());
        expect(evidence.operationClaims[operation.operationId].status).toBe('dated-live-observation');
      } else {
        expect(Object.keys(operation.responses)).toEqual(['default']);
        expect(evidence.operationClaims[operation.operationId].status).toBe('explicit-unknown');
      }
    }
  });

  it('uses the observed nested text-blast message and excludes the superseded shape', () => {
    const data = spec.paths['/createTextBlast'].post.requestBody
      .content['application/json'].schema.properties.data;
    const params = data.properties.params;
    expect(params.properties.message).toMatchObject({
      type: 'object',
      required: expect.arrayContaining(['text', 'to', 'showOnEventPage']),
    });
    expect(data.properties.userId).toMatchObject({ type: 'string' });
    expect(params.properties).not.toHaveProperty('recipientStatuses');
    expect(evidence.contradictions.find(({ id }) => id === 'text-blast-message-shape')?.status).toBe('resolved');
  });

  it('keeps unknown default responses free of success-body schemas', () => {
    const observedResponseOps = new Set([
      'getPosterCatalog',
      'sendAuthCodeTrusted',
      'getLoginToken',
      'signInWithCustomToken',
      'refreshToken',
      'lookupFirebaseUser',
    ]);
    const observedResponseSources = new Map([
      ['getPosterCatalog', evidence.sources.posterCatalogObservation],
      ['sendAuthCodeTrusted', evidence.sources.authSendCode20260811],
      ['getLoginToken', evidence.sources.authLoginToken20260811],
      ['signInWithCustomToken', evidence.sources.authSignIn20260811],
      ['refreshToken', evidence.sources.authRefresh20260811],
      ['lookupFirebaseUser', evidence.sources.authLookup20260811],
    ]);
    for (const { path, method, operation } of operations()) {
      const base = `#/paths/${escapePointerSegment(path)}/${method}/responses/default`;
      expect(evidence.claims[base].citation).toBe(evidence.sources.unknownStatusDecision);
      expect(operation.responses.default).not.toHaveProperty('content');
      const operationClaim = evidence.operationClaims[operation.operationId];
      if (observedResponseOps.has(operation.operationId)) {
        expect(operationClaim.response).toBe('dated-live-observation');
        expect(citationResolves(operationClaim.responseCitation), `${operation.operationId} responseCitation`).toBe(true);
        expect(operationClaim.responseCitation)
          .toBe(observedResponseSources.get(operation.operationId));
      } else {
        expect(operationClaim.response).toBe('explicit-unknown');
        expect(operationClaim.responseCitation).toBe(evidence.sources.unknownStatusDecision);
      }
    }
  });

  it('uses each Markdown decision source only for its stated claim category', () => {
    for (const [pointer, claim] of Object.entries(evidence.claims)) {
      if (claim.citation === evidence.sources.unknownStatusDecision) {
        expect(pointer).toMatch(/\/responses\/default$/);
      }
      if (claim.citation === evidence.sources.textBlast) {
        expect(pointer).toContain('~1createTextBlast/post/requestBody');
      }
      if (claim.citation === evidence.sources.updateMask) {
        expect(pointer).toMatch(/updateMask\.fieldPaths|parameters\/1\/(?:style|explode)$/);
      }
      if (claim.citation === evidence.sources.posterCatalogObservation) {
        expect(pointer.includes('~1posters.json') || pointer.includes('/schemas/Poster')).toBe(true);
      }
      const authCitationScopes = new Map([
        [evidence.sources.authSendCode20260811, ['~1sendAuthCodeTrusted']],
        [evidence.sources.authLoginToken20260811, ['~1getLoginToken', 'LoginTokenResponse', 'CallableAuthError']],
        [evidence.sources.authSignIn20260811, ['signInWithCustomToken', 'FirebaseSignInResponse', 'FirebaseValidationError']],
        [evidence.sources.authRefresh20260811, ['~1token', 'RefreshTokenResponse', 'FirebaseTokenError']],
        [evidence.sources.authLookup20260811, ['accounts:lookup', 'FirebaseLookup', 'FirebaseProviderUserInfo']],
      ]);
      if (authCitationScopes.has(claim.citation)) {
        expect(
          authCitationScopes.get(claim.citation).some((fragment) => pointer.includes(fragment)),
          `auth citation scope: ${pointer}`,
        ).toBe(true);
      }
    }
  });

  it('covers every security assignment, including empty scopes', () => {
    for (const { path, method, operation } of operations()) {
      if (!operation.security) continue;
      expect(operation.security.length, operation.operationId).toBeGreaterThan(0);
      for (let index = 0; index < operation.security.length; index++) {
        for (const [scheme, scopes] of Object.entries(operation.security[index])) {
          const pointer = `#/paths/${escapePointerSegment(path)}/${method}/security/${index}/${scheme}`;
          expect(Array.isArray(scopes), pointer).toBe(true);
          expect(evidence.claims[pointer], pointer).toBeDefined();
        }
      }
    }
  });

  it('models update masks as repeated query values and leaves undocumented localId absent', () => {
    const patch = spec.paths[
      '/v1/projects/getpartiful/databases/(default)/documents/events/{eventId}'
    ].patch;
    const updateMask = patch.parameters.find(({ name }) => name === 'updateMask.fieldPaths');
    expect(updateMask).toMatchObject({
      in: 'query',
      schema: { type: 'array', items: { type: 'string' } },
      style: 'form',
      explode: true,
    });
    expect(spec.components.schemas.FirebaseSignInResponse.properties ?? {})
      .not.toHaveProperty('localId');
  });

  it('does not claim client defaults or client-specific behavior as remote facts', () => {
    const list = spec.paths[
      '/v1/projects/getpartiful/databases/(default)/documents/{collectionPath}'
    ].get;
    expect(list.parameters.find(({ name }) => name === 'pageSize').schema)
      .not.toHaveProperty('default');
    expect(JSON.stringify(spec).toLowerCase()).not.toContain('custom client');
  });

  it('keeps the human operation summary aligned with machine classifications', () => {
    const ledger = fs.readFileSync(
      'docs/research/2026-08-10-partiful-api-contract-evidence-ledger.md',
      'utf8',
    );
    const knownOperationIds = new Set(Object.keys(evidence.operations));
    const documentedOperations = (heading) => {
      const marker = `### ${heading}\n`;
      const start = ledger.indexOf(marker);
      expect(start, heading).toBeGreaterThanOrEqual(0);
      const section = ledger.slice(start + marker.length).split(/\n#{2,6} /)[0];
      return [...section.matchAll(/`([^`]+)`/g)]
        .map((match) => match[1])
        .filter((operationId) => knownOperationIds.has(operationId))
        .sort();
    };
    for (const [heading, classification] of [
      ['Dated-live operations', 'dated-live-observation'],
      ['TypeScript-derived operations', 'typescript-derived-inference'],
    ]) {
      const machineOperations = Object.entries(evidence.operations)
        .filter(([, operation]) => operation.classification === classification)
        .map(([operationId]) => operationId)
        .sort();
      expect(documentedOperations(heading)).toEqual(machineOperations);
    }
  });

  it('rejects missing and mismatched historical evidence values', () => {
    expect(jsonPointerValue(historicalDraft, '#/not-a-real-pointer')).toBeUndefined();
    expect(jsonPointerValue(historicalDraft, '#/openapi')).not.toEqual(spec.info.version);
  });

  it('records the observed poster catalog without borrowing event-image evidence', () => {
    const posterOperation = evidence.operations.getPosterCatalog;
    expect(posterOperation.classification).toBe('dated-live-observation');
    expect(posterOperation.citation).toBe(evidence.sources.posterCatalogObservation);
    expect(evidence.sources).not.toHaveProperty('eventImageObservation');
    for (const [pointer, claim] of Object.entries(evidence.claims)) {
      if (pointer.includes('/posters.json') || pointer.includes('/Poster')) {
        expect(claim.citation).not.toBe(evidence.sources.eventImageObservation);
      }
    }
  });

  it('captures the complete poster response needed for bounded local pagination', () => {
    const operation = spec.paths['/posters.json'].get;
    expect(operation.responses['200']).toEqual({
      description: 'Complete poster catalog.',
      content: {
        'application/json': {
          schema: {
            type: 'array',
            items: { $ref: '#/components/schemas/Poster' },
          },
        },
      },
    });
    expect(operation.responses.default).not.toHaveProperty('content');
    expect(evidence.claims['#/paths/~1posters.json/get/operationId']).toEqual({
      classification: 'typescript-derived-inference',
      citation:
        'spec/research/historical-27-operation-draft.json#/paths/~1posters.json/get/operationId',
    });

    const poster = spec.components.schemas.Poster;
    expect(poster.required).toEqual([
      'id',
      'name',
      'url',
      'contentType',
      'width',
      'height',
      'tags',
      'categories',
    ]);
    expect(poster.properties.width.type).toEqual(['integer', 'null']);
    expect(poster.properties.height.type).toEqual(['integer', 'null']);
    expect(poster.properties.tags.items.type).toBe('string');
    expect(poster.properties.categories.items.type).toBe('string');
    for (const pointer of [
      '#/components/schemas/Poster/properties/blurHash',
      '#/components/schemas/Poster/properties/blurHash/type',
      '#/components/schemas/Poster/additionalProperties',
    ]) {
      expect(evidence.claims[pointer]).toEqual({
        classification: 'typescript-derived-inference',
        citation: evidence.sources.posterInterface,
      });
    }

    expect(evidence.posterCatalogObservation).toMatchObject({
      sourceCitation: 'docs/research/2026-08-11-poster-catalog-observation.md#scope-and-provenance',
      observedAt: '2026-08-11T01:08:30Z',
      status: 200,
      mediaType: 'application/json',
      topLevel: 'array',
      itemCount: 2114,
      payloadBytes: 1125932,
      payloadSha256: '35e22005b19dd5795cecf582dee4c4fe4ddc5349e3142f0aae8014f4e471cc6e',
      completeRepresentation: true,
      allProductFieldsPresent: true,
      duplicateIdEntries: 1,
      contentTypeVerifiedAt: '2026-08-11T01:42:58Z',
      contentTypes: ['image/avif', 'image/gif', 'image/jpeg', 'image/png'],
    });
    expect(citationResolves(evidence.posterCatalogObservation.sourceCitation)).toBe(true);
    expect(evidence.posterCatalogObservation.pagination).toEqual({
      remotePagination: false,
      localPagination: 'full-representation',
      resumeBinding: 'payload-sha256-normalized-filters-and-next-offset',
    });
    expect(evidence.posterCatalogObservation.failureObservation).toEqual({
      requestCondition: 'unsatisfiable-byte-range',
      status: 416,
      bodyBytes: 0,
    });
    expect(evidence.posterCatalogObservation.failureBoundary).toEqual({
      noResponse: 'remote.unavailable',
      receivedNon200: 'contract.protocol_changed',
      malformedSuccess: 'contract.protocol_changed',
      rateLimiting: 'not-claimed',
    });
  });

  it('captures the observed authentication response shapes from the attended session', () => {
    // Load and verify both committed sanitized artifacts independently
    const artifactPath = evidence.authObservation.artifactPath;
    expect(fs.existsSync(artifactPath)).toBe(true);
    const artifact = JSON.parse(fs.readFileSync(artifactPath, 'utf8'));
    expect(artifact.redaction).toContain('HTTP metadata');

    const probeArtifactPath = evidence.authObservation.probeArtifactPath;
    expect(fs.existsSync(probeArtifactPath)).toBe(true);
    const probeArtifact = JSON.parse(fs.readFileSync(probeArtifactPath, 'utf8'));
    expect(probeArtifact.probeMethod).toContain('fake tokens');

    expect(evidence.authObservation).toMatchObject({
      sourceCitation: 'docs/research/2026-08-11-auth-observation.md#scope-and-provenance',
      observedAt: '2026-08-11T02:30:19Z',
    });
    expect(citationResolves(evidence.authObservation.sourceCitation)).toBe(true);

    // Map artifact observations by operation+phase for independent verification
    const byOpPhase = new Map();
    for (const obs of artifact.observations) {
      byOpPhase.set(`${obs.operation}:${obs.phase}`, obs);
    }
    for (const obs of probeArtifact.observations) {
      byOpPhase.set(`probe:${obs.operation}:${obs.phase}`, obs);
    }

    // Helper: navigate schema following $refs and verify artifact shape paths
    function verifyShapeAgainstSchema(operationId, schema, shape) {
      for (const { path: shapePath, type: shapeType } of shape) {
        const segments = shapePath.split('.');
        let current = schema;
        for (const seg of segments) {
          if (seg === '[]') {
            current = current.items;
            if (current?.$ref) current = spec.components.schemas[current.$ref.split('/').pop()];
          } else {
            if (current?.$ref) current = spec.components.schemas[current.$ref.split('/').pop()];
            current = current?.properties?.[seg];
          }
          expect(current, `${operationId} schema path ${shapePath} at ${seg}`).toBeDefined();
        }
        if (current?.$ref) current = spec.components.schemas[current.$ref.split('/').pop()];
        if (current?.type) expect(current.type, `${operationId} ${shapePath} type`).toBe(shapeType);
      }
    }

    // Verify each auth operation's success response matches the artifact
    const authOps = [
      { id: 'sendAuthCodeTrusted', path: '/sendAuthCodeTrusted' },
      { id: 'getLoginToken', path: '/getLoginToken' },
      { id: 'signInWithCustomToken', path: '/v1/accounts:signInWithCustomToken' },
      { id: 'refreshToken', path: '/v1/token' },
      { id: 'lookupFirebaseUser', path: '/v1/accounts:lookup' },
    ];

    for (const { id, path: opPath } of authOps) {
      const success = byOpPhase.get(`${id}:success-attempt`);
      expect(success, `${id} must have success-attempt in artifact`).toBeDefined();
      expect(success.status, `${id} artifact status`).toBe(200);

      const operation = spec.paths[opPath].post;
      expect(Object.keys(operation.responses), `${id} retains default`).toContain('default');
      expect(Object.keys(operation.responses), `${id} has 200`).toContain('200');

      expect(evidence.operations[id].classification, `${id} operation class`).toBe('dated-live-observation');
      const claim = evidence.operationClaims[id];
      expect(claim.response, `${id} response class`).toBe('dated-live-observation');
      expect(claim.status, `${id} status class`).toBe('dated-live-observation');
      const expectedCitation = new Map([
        ['sendAuthCodeTrusted', evidence.sources.authSendCode20260811],
        ['getLoginToken', evidence.sources.authLoginToken20260811],
        ['signInWithCustomToken', evidence.sources.authSignIn20260811],
        ['refreshToken', evidence.sources.authRefresh20260811],
        ['lookupFirebaseUser', evidence.sources.authLookup20260811],
      ]).get(id);
      expect(claim.responseCitation, `${id} responseCitation`).toBe(expectedCitation);

      // Request provenance preserved
      if (['sendAuthCodeTrusted', 'refreshToken', 'lookupFirebaseUser'].includes(id)) {
        expect(claim.request, `${id} request class preserved`).toBe('typescript-derived-inference');
      }

      // Verify 200 response shape against artifact (except sendAuthCodeTrusted which has no content)
      if (id !== 'sendAuthCodeTrusted' && success.shape.length > 0) {
        const ref200 = operation.responses['200'].content['application/json'].schema;
        const schemaName = ref200.$ref?.split('/').pop();
        const schema = schemaName ? spec.components.schemas[schemaName] : ref200;
        verifyShapeAgainstSchema(id, schema, success.shape);
      }
    }

    // sendAuthCodeTrusted 200 has no content (body shape unclaimed)
    expect(spec.paths['/sendAuthCodeTrusted'].post.responses['200'])
      .not.toHaveProperty('content');

    // Verify observed error responses against artifacts
    // getLoginToken 403: owner artifact wrong-code observation
    const login403 = byOpPhase.get('getLoginToken:wrong-code');
    expect(login403).toBeDefined();
    expect(login403.status).toBe(403);
    const login403Schema = spec.paths['/getLoginToken'].post.responses['403']
      .content['application/json'].schema;
    const login403Ref = spec.components.schemas[login403Schema.$ref.split('/').pop()];
    verifyShapeAgainstSchema('getLoginToken-403', login403Ref, login403.shape);

    // signInWithCustomToken 400: agent probe artifact
    const signIn400 = byOpPhase.get('probe:signInWithCustomToken:invalid-token');
    expect(signIn400).toBeDefined();
    expect(signIn400.status).toBe(400);
    const signIn400Schema = spec.paths['/v1/accounts:signInWithCustomToken'].post.responses['400']
      .content['application/json'].schema;
    const signIn400Ref = spec.components.schemas[signIn400Schema.$ref.split('/').pop()];
    verifyShapeAgainstSchema('signInWithCustomToken-400', signIn400Ref, signIn400.shape);

    // refreshToken 400: owner artifact + agent probe (both observed)
    const refresh400Owner = byOpPhase.get('refreshToken:invalid-token');
    expect(refresh400Owner).toBeDefined();
    expect(refresh400Owner.status).toBe(400);
    const refresh400Schema = spec.paths['/v1/token'].post.responses['400']
      .content['application/json'].schema;
    const refresh400Ref = spec.components.schemas[refresh400Schema.$ref.split('/').pop()];
    verifyShapeAgainstSchema('refreshToken-400', refresh400Ref, refresh400Owner.shape);

    // lookupFirebaseUser 400: owner artifact + agent probe (both observed)
    const lookup400Owner = byOpPhase.get('lookupFirebaseUser:invalid-token');
    expect(lookup400Owner).toBeDefined();
    expect(lookup400Owner.status).toBe(400);
    const lookup400Schema = spec.paths['/v1/accounts:lookup'].post.responses['400']
      .content['application/json'].schema;
    const lookup400Ref = spec.components.schemas[lookup400Schema.$ref.split('/').pop()];
    verifyShapeAgainstSchema('lookupFirebaseUser-400', lookup400Ref, lookup400Owner.shape);

    // additionalProperties on all auth schemas must NOT be dated-live-observation
    const authSchemas = [
      'LoginTokenResponse', 'FirebaseSignInResponse', 'RefreshTokenResponse',
      'FirebaseLookupResponse', 'FirebaseLookupUser', 'FirebaseProviderUserInfo',
      'CallableAuthError', 'FirebaseValidationError', 'FirebaseTokenError',
    ];
    for (const name of authSchemas) {
      const apPointer = `#/components/schemas/${name}/additionalProperties`;
      if (evidence.claims[apPointer]) {
        expect(evidence.claims[apPointer].classification, `${name} additionalProperties`)
          .toBe('typescript-derived-inference');
      }
    }

    // required arrays: fields must be present in the artifact success observation
    function requiredFieldsObserved(schemaName, artifactOp) {
      const schema = spec.components.schemas[schemaName];
      if (!schema?.required) return;
      const success = byOpPhase.get(`${artifactOp}:success-attempt`);
      for (const field of schema.required) {
        const found = success.shape.some(({ path }) => path.startsWith(field) || path === field);
        expect(found, `${schemaName}.required field '${field}' observed in artifact`).toBe(true);
      }
    }
    requiredFieldsObserved('FirebaseSignInResponse', 'signInWithCustomToken');
    requiredFieldsObserved('RefreshTokenResponse', 'refreshToken');
    requiredFieldsObserved('FirebaseLookupResponse', 'lookupFirebaseUser');
    const loginSuccess = byOpPhase.get('getLoginToken:success-attempt');
    expect(loginSuccess.shape.some(({ path }) => path.startsWith('result'))).toBe(true);

    // Operation-specific failure boundaries
    const fb = evidence.authObservation.failureBoundary;

    // getLoginToken: 403 -> input.invalid / AUTH_CODE_REJECTED
    expect(fb.getLoginToken['200']).toBe('success');
    expect(fb.getLoginToken['403'].productMapping).toBe('input.invalid');
    expect(fb.getLoginToken['403'].stableCode).toBe('AUTH_CODE_REJECTED');
    expect(fb.getLoginToken.otherReceived).toBe('contract.protocol_changed');
    expect(fb.getLoginToken.noResponse).toBe('remote.unavailable');
    expect(fb.getLoginToken.rateLimiting).toBe('not-claimed');

    // signInWithCustomToken: 400 -> auth.expired
    expect(fb.signInWithCustomToken['200']).toBe('success');
    expect(fb.signInWithCustomToken['400'].productMapping).toBe('auth.expired');
    expect(fb.signInWithCustomToken['400'].stableCode).toBe('INVALID_CUSTOM_TOKEN');
    expect(fb.signInWithCustomToken.otherReceived).toBe('contract.protocol_changed');

    // refreshToken: 400 -> auth.expired
    expect(fb.refreshToken['200']).toBe('success');
    expect(fb.refreshToken['400'].productMapping).toBe('auth.expired');
    expect(fb.refreshToken['400'].stableCode).toBe('INVALID_REFRESH_TOKEN');
    expect(fb.refreshToken.otherReceived).toBe('contract.protocol_changed');

    // lookupFirebaseUser: 400 -> auth.expired, optional for S2
    expect(fb.lookupFirebaseUser['200']).toBe('success');
    expect(fb.lookupFirebaseUser['400'].productMapping).toBe('auth.expired');
    expect(fb.lookupFirebaseUser.s2Gate).toBe(false);

    // sendAuthCodeTrusted: no error status promoted
    expect(fb.sendAuthCodeTrusted['200']).toBe('success');
    expect(fb.sendAuthCodeTrusted).not.toHaveProperty('400');
    expect(fb.sendAuthCodeTrusted).not.toHaveProperty('403');
    expect(fb.sendAuthCodeTrusted.otherReceived).toBe('contract.protocol_changed');
  });
});
