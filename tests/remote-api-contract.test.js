import { describe, expect, it } from 'vitest';
import evidence from '../spec/partiful.api-evidence.json';
import spec from '../spec/partiful.openapi.json';

const methods = new Set(['get', 'post', 'put', 'patch', 'delete']);
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

describe('remote API contract', () => {
  it('is a versioned OpenAPI 3.1 document with unique operation IDs', () => {
    expect(spec.openapi).toBe('3.1.0');
    expect(spec.info.version).toBe('1.0.0');
    const ids = operations().map(({ operation }) => operation.operationId);
    expect(ids).toHaveLength(27);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('covers every operation and material schema with allowed evidence', () => {
    const allowed = new Set(evidence.allowedClassifications);
    const operationEvidence = new Map(evidence.operations);
    for (const { operation } of operations()) {
      expect(allowed.has(operationEvidence.get(operation.operationId))).toBe(true);
      const claim = evidence.operationClaims[operation.operationId] ?? evidence.operationClaims.default;
      expect(allowed.has(claim.request)).toBe(true);
      expect(allowed.has(claim.response)).toBe(true);
    }
    for (const schemaName of Object.keys(spec.components.schemas)) {
      const claim = evidence.schemaClaims[schemaName];
      expect(claim, schemaName).toBeDefined();
      expect(allowed.has(claim.classification), schemaName).toBe(true);
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

  it('uses the observed nested text-blast message and excludes the superseded shape', () => {
    const params = spec.components.schemas.TextBlastRequest.properties.data.properties.params;
    expect(params.properties.message.$ref).toBe('#/components/schemas/TextBlastMessage');
    expect(params.properties).not.toHaveProperty('recipientStatuses');
    expect(spec.components.schemas.TextBlastMessage.required).toEqual(
      expect.arrayContaining(['text', 'to', 'showOnEventPage']),
    );
    expect(evidence.contradictions.find(({ id }) => id === 'text-blast-message-shape')?.status).toBe('resolved');
  });
});
