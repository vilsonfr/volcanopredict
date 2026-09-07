import { useEffect, useRef, useState } from "react";
import { searchVolcanoes, type Volcano } from "../api/client";

/**
 * Busca de vulcões por nome ou país.
 *
 * A consulta vai ao servidor a cada digitação (com atraso), em vez de filtrar
 * a lista já carregada: é a API que define o que é um resultado válido, e o
 * mesmo comportamento continuará valendo quando o catálogo crescer para além
 * do que cabe na memória do navegador.
 */
export function SearchPanel({
  onSelect,
}: {
  onSelect: (volcano: Volcano) => void;
}) {
  const [term, setTerm] = useState("");
  const [results, setResults] = useState<Volcano[]>([]);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Descarta respostas de buscas antigas que chegarem fora de ordem: sem isso,
  // uma consulta lenta pode sobrescrever o resultado de uma mais recente.
  const seqRef = useRef(0);

  useEffect(() => {
    const trimmed = term.trim();
    if (trimmed.length < 2) {
      setResults([]);
      setError(null);
      setSearching(false);
      return;
    }

    const seq = ++seqRef.current;
    setSearching(true);
    const timer = setTimeout(async () => {
      try {
        const found = await searchVolcanoes(trimmed, 20);
        if (seq !== seqRef.current) return;
        setResults(found);
        setError(null);
      } catch (e) {
        if (seq !== seqRef.current) return;
        setError(e instanceof Error ? e.message : String(e));
        setResults([]);
      } finally {
        if (seq === seqRef.current) setSearching(false);
      }
    }, 250);

    return () => clearTimeout(timer);
  }, [term]);

  const trimmed = term.trim();

  return (
    <div className="search">
      <input
        type="search"
        value={term}
        onChange={(e) => setTerm(e.target.value)}
        placeholder="Buscar vulcão ou país…"
        aria-label="Buscar vulcão ou país"
      />

      {error && <p className="search__msg search__msg--error">{error}</p>}

      {!error && trimmed.length >= 2 && !searching && results.length === 0 && (
        <p className="search__msg">Nenhum vulcão encontrado.</p>
      )}

      {results.length > 0 && (
        <ul className="search__results">
          {results.map((v) => (
            <li key={v.id}>
              <button type="button" onClick={() => onSelect(v)}>
                <span className="search__name">{v.name}</span>
                <span className="search__country">{v.country}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
