/**
 * Cliente da API pública.
 *
 * Todo dado exibido pela interface vem daqui. Nada é embutido no HTML nem
 * mantido em memória no frontend como fonte de verdade — a API é a única
 * origem, o que mantém o contrato testável e a proveniência intacta.
 */

const BASE_URL = (
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080"
).replace(/\/+$/, "");

export type Volcano = {
  id: number;
  name: string;
  country?: string;
  latitude: number;
  longitude: number;
  elevation_m?: number;
  status?: string;
  source_ref: string;
  absent_from_source?: boolean;
  distance_m?: number;
};

export type Attribution = {
  source: string;
  license?: string;
  attribution?: string;
  url?: string;
};

type Envelope<T> = {
  data: T;
  meta: { count: number; next_cursor?: string; disclaimer?: string };
  attribution?: Attribution[];
};

export class ApiError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function get<T>(path: string, params: Record<string, string> = {}) {
  const url = new URL(BASE_URL + path);
  for (const [k, v] of Object.entries(params)) url.searchParams.set(k, v);

  const res = await fetch(url, { headers: { Accept: "application/json" } });
  const body = await res.json().catch(() => null);

  if (!res.ok) {
    // A API sempre devolve erro no formato padrão; o request_id permite
    // correlacionar com o log do servidor.
    const err = body?.error;
    throw new ApiError(
      err?.message ?? `Falha na requisição (HTTP ${res.status})`,
      err?.code ?? "unknown",
      err?.request_id,
    );
  }
  return body as Envelope<T>;
}

/**
 * Carrega o catálogo inteiro, seguindo o cursor de paginação até o fim.
 *
 * A paginação é keyset: cada página devolve o cursor da próxima, e a ausência
 * de cursor significa fim da coleção. O limite por página é o máximo que a API
 * aceita, para minimizar idas ao servidor.
 */
export async function fetchAllVolcanoes(
  onProgress?: (loaded: number) => void,
): Promise<{ volcanoes: Volcano[]; attribution: Attribution[] }> {
  const volcanoes: Volcano[] = [];
  let attribution: Attribution[] = [];
  let cursor: string | undefined;

  // Trava de segurança: sem ela, um cursor que não avança viraria laço
  // infinito no navegador em vez de um erro visível.
  for (let page = 0; page < 100; page++) {
    const params: Record<string, string> = { limit: "500" };
    if (cursor) params.cursor = cursor;

    const res = await get<Volcano[]>("/api/v1/volcanoes", params);
    volcanoes.push(...res.data);
    if (res.attribution?.length) attribution = res.attribution;
    onProgress?.(volcanoes.length);

    if (!res.meta.next_cursor) return { volcanoes, attribution };
    cursor = res.meta.next_cursor;
  }
  throw new ApiError(
    "A paginação do catálogo não terminou no número esperado de páginas.",
    "pagination_overflow",
  );
}
