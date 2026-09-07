import { useEffect, useRef, useState } from "react";
import {
  Viewer,
  Cartesian3,
  Math as CesiumMath,
  ShadowMode,
  Color,
  NearFarScalar,
  ImageryLayer,
  ScreenSpaceEventHandler,
  ScreenSpaceEventType,
  SceneTransforms,
  Cartesian2,
  Ellipsoid,
  defined,
} from "cesium";
import { fetchAllVolcanoes, type Volcano } from "../api/client";
import { VolcanoCallout } from "./VolcanoCallout";
import "cesium/Build/Cesium/Widgets/widgets.css";
import { buildImagery } from "./imagery";

type Status = "loading" | "ready" | "error";

/**
 * Reporta um problema de WebGL em linguagem acionavel, ou null se estiver tudo
 * bem.
 *
 * Vale checar antes de instanciar o Viewer: o Cesium falha no meio do laco de
 * renderizacao, com uma mensagem que nao diz o que fazer a respeito.
 */
function detectWebGLProblem(): string | null {
  const canvas = document.createElement("canvas");
  const gl = (canvas.getContext("webgl2") ??
    canvas.getContext("webgl")) as WebGLRenderingContext | null;

  if (!gl) {
    return (
      "Este navegador não forneceu um contexto WebGL. Verifique se a " +
      "aceleração de hardware está habilitada nas configurações do navegador."
    );
  }

  const maxTexture = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
  if (!maxTexture || maxTexture <= 0) {
    return (
      "O contexto WebGL foi criado sem capacidade de textura " +
      "(MAX_TEXTURE_SIZE = 0). Isso costuma acontecer quando há contextos " +
      "WebGL demais abertos: recarregue a página (Ctrl+Shift+R) e feche " +
      "outras abas que usem 3D. Se persistir, verifique a aceleração de " +
      "hardware do navegador."
    );
  }

  // Libera o contexto de teste imediatamente: ele conta para o limite do
  // navegador, e segura-lo tornaria o proprio diagnostico parte do problema.
  gl.getExtension("WEBGL_lose_context")?.loseContext();
  return null;
}

/** Acoes que o painel pode disparar sobre o globo. */
export type GlobeControls = {
  /** Aproxima a camera de um vulcao e abre seu balao. */
  focus: (volcano: Volcano) => void;
  /** Liga ou desliga a camada de vulcoes. */
  setVolcanoesVisible: (visible: boolean) => void;
};

export function Globe({
  onTerrainResolved,
  onVolcanoesLoaded,
  onReady,
}: {
  onTerrainResolved?: (hasRealTerrain: boolean) => void;
  onVolcanoesLoaded?: (count: number, error?: string) => void;
  onReady?: (controls: GlobeControls) => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewerRef = useRef<Viewer | null>(null);
  const [status, setStatus] = useState<Status>("loading");
  const [error, setError] = useState<string | null>(null);
  const [imageryWarning, setImageryWarning] = useState<string | null>(null);
  // Vulcao selecionado e sua posicao em pixels, recalculada a cada quadro.
  const [selected, setSelected] = useState<{
    volcano: Volcano;
    x: number;
    y: number;
  } | null>(null);
  const byIdRef = useRef<Map<number, Volcano>>(new Map());
  const selectedIdRef = useRef<number | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    let viewer: Viewer | null = null;
    let cancelled = false;

    // Um Viewer por container, sempre. O StrictMode monta o efeito duas vezes
    // e o HMR remonta a cada edicao; sem esta guarda cada ciclo cria um
    // contexto WebGL novo. O navegador limita quantos existem ao mesmo tempo
    // (tipicamente 16) e, passado o limite, devolve um contexto morto — cujo
    // MAX_TEXTURE_SIZE e 0, que e exatamente o erro
    // "Width must be less than or equal to the maximum texture size (0)".
    if (viewerRef.current && !viewerRef.current.isDestroyed()) {
      return;
    }

    (async () => {
      try {
        const webglProblem = detectWebGLProblem();
        if (webglProblem) {
          setError(webglProblem);
          setStatus("error");
          return;
        }

        const { imageryProvider, terrainProvider, hasRealTerrain } =
          await buildImagery();
        if (cancelled) return;

        viewer = new Viewer(container, {
          baseLayerPicker: false,
          geocoder: false,
          homeButton: false,
          sceneModePicker: false,
          navigationHelpButton: false,
          animation: false,
          timeline: false,
          fullscreenButton: false,
          infoBox: false,
          selectionIndicator: false,
          terrainProvider,
          contextOptions: {
            // WebGL 2 explicito. Sem isto o Cesium pede WebGL 1, e em
            // ambientes onde so ha WebGL 2 ele recebe um contexto degradado
            // cujo MAX_TEXTURE_SIZE e 0 — o que produz exatamente
            // "Width must be less than or equal to the maximum texture
            // size (0)" e um globo preto, sem nenhuma pista da causa.
            // Um contexto com caveat de performance (renderizacao por
            // software) ainda desenha o globo; recusa-lo trocaria uma tela
            // lenta por uma tela vazia.
            webgl: { failIfMajorPerformanceCaveat: false },
          },
        });
        viewerRef.current = viewer;

        // ImageryLayer explicito: addImageryProvider e a API antiga e nao
        // garante que a camada entre quando o provider ainda esta se
        // inicializando — o sintoma e um globo cinza sem textura.
        viewer.imageryLayers.removeAll();
        viewer.imageryLayers.add(new ImageryLayer(imageryProvider));

        const { scene } = viewer;
        // Ajustes que separam "esfera com textura" de algo que parece a Terra:
        // sombreamento solar real, atmosfera, e névoa com a distância.
        // Iluminacao solar desligada de proposito. Com ela ligada o globo
        // fica preto neste ambiente — e, mesmo funcionando, deixaria metade
        // dos vulcoes na sombra a qualquer momento. Para uma ferramenta de
        // monitoramento isso e uma perda: o objetivo e ver todos os vulcoes,
        // nao so os que estao no lado iluminado agora.
        scene.globe.enableLighting = false;
        scene.globe.showGroundAtmosphere = true;
        scene.globe.depthTestAgainstTerrain = true;
        if (scene.skyAtmosphere) scene.skyAtmosphere.show = true;
        scene.fog.enabled = true;
        scene.shadowMap.enabled = false;
        scene.globe.shadows = ShadowMode.DISABLED;
        scene.backgroundColor = Color.BLACK;
        scene.highDynamicRange = false;

        viewer.camera.setView({
          // Vista inicial sobre o Circulo de Fogo do Pacifico, onde esta a
          // maior concentracao de vulcoes do catalogo.
          destination: Cartesian3.fromDegrees(140, 5, 24_000_000),
          orientation: {
            heading: 0,
            pitch: CesiumMath.toRadians(-90),
            roll: 0,
          },
        });

        // O contexto do proprio Cesium e o que importa: a checagem previa usa
        // um canvas separado, que pode estar saudavel enquanto este nao esta.
        const ctxTextureSize = (
          viewer.scene as unknown as {
            context?: { maximumTextureSize?: number };
          }
        ).context?.maximumTextureSize;
        if (ctxTextureSize !== undefined && ctxTextureSize <= 0) {
          setError(
            "O contexto WebGL do globo foi criado sem capacidade de textura " +
              "(MAX_TEXTURE_SIZE = 0). Recarregue a página; se persistir, " +
              "verifique a aceleração de hardware do navegador.",
          );
          setStatus("error");
          return;
        }

        // Erro dentro do laco de renderizacao: sem isto o Cesium para de
        // desenhar e a tela fica preta sem explicacao nenhuma.
        viewer.scene.renderError.addEventListener((_scene, err) => {
          setError(
            "Falha ao renderizar o globo: " +
              (err instanceof Error ? err.message : String(err)),
          );
          setStatus("error");
        });

        // Falha ao buscar tiles da imagem de satelite. Nao impede o resto de
        // funcionar, mas precisa ser dita — e a diferenca entre "a Terra esta
        // preta porque a fonte falhou" e "esta preta e ninguem sabe por que".
        let tileErrors = 0;
        imageryProvider.errorEvent.addEventListener((e) => {
          tileErrors++;
          if (tileErrors === 1) {
            console.warn("Falha ao carregar tile de imagem de satelite:", e);
            setImageryWarning(
              "A imagem de satélite não carregou. O globo funciona, mas sem " +
                "textura de superfície.",
            );
          }
        });

        viewer.scene.canvas.addEventListener("webglcontextlost", (ev) => {
          ev.preventDefault();
          setError(
            "O navegador perdeu o contexto WebGL. Recarregue a página para " +
              "voltar a renderizar o globo.",
          );
          setStatus("error");
        });

        onTerrainResolved?.(hasRealTerrain);
        setStatus("ready");

        // O catalogo vem da API, nunca embutido no bundle. Se a API estiver
        // fora, o globo continua utilizavel e o erro aparece no painel — em
        // vez de a tela inteira falhar.
        try {
          const { volcanoes } = await fetchAllVolcanoes();
          if (cancelled || viewer.isDestroyed()) return;
          plotVolcanoes(viewer, volcanoes);
          byIdRef.current = new Map(volcanoes.map((v) => [v.id, v]));
          wireSelection(viewer, byIdRef.current, selectedIdRef, setSelected);
          onVolcanoesLoaded?.(volcanoes.length);

          const v = viewer;
          onReady?.({
            focus: (volcano) => {
              selectedIdRef.current = volcano.id;
              v.camera.flyTo({
                destination: Cartesian3.fromDegrees(
                  volcano.longitude,
                  volcano.latitude,
                  // Altura que mostra o vulcao e o entorno; perto o bastante
                  // para dar contexto geografico, longe o bastante para nao
                  // parecer uma textura borrada.
                  450_000,
                ),
                duration: 1.6,
              });
            },
            setVolcanoesVisible: (visible) => {
              for (const entity of v.entities.values) {
                entity.show = visible;
              }
              if (!visible) {
                selectedIdRef.current = null;
                setSelected(null);
              }
            },
          });
        } catch (e) {
          if (cancelled) return;
          onVolcanoesLoaded?.(0, e instanceof Error ? e.message : String(e));
        }
      } catch (err) {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : String(err));
        setStatus("error");
      }
    })();

    return () => {
      cancelled = true;
      if (viewer && !viewer.isDestroyed()) viewer.destroy();
      viewerRef.current = null;
    };
  }, [onTerrainResolved, onVolcanoesLoaded, onReady]);

  return (
    <div className="globe-root">
      <div ref={containerRef} className="globe-canvas" />
      {selected && (
        <VolcanoCallout
          volcano={selected.volcano}
          x={selected.x}
          y={selected.y}
          onClose={() => {
            selectedIdRef.current = null;
            setSelected(null);
          }}
        />
      )}
      {status === "loading" && (
        <div className="globe-overlay">Carregando o globo…</div>
      )}
      {imageryWarning && status === "ready" && (
        <div className="globe-warning">{imageryWarning}</div>
      )}
      {status === "error" && (
        <div className="globe-overlay globe-overlay--error">
          Não foi possível iniciar o globo: {error}
        </div>
      )}
    </div>
  );
}

/**
 * Desenha o catalogo no globo.
 *
 * Cada vulcao vira um ponto cujo raio cresce ao aproximar. Deliberadamente sem
 * rotulo por entidade: 1.214 texturas de texto simultaneas sao ilegiveis na
 * visao global e pesam o bastante para atrasar o carregamento dos tiles. O
 * nome vem pelo clique, via `description`.
 */
function plotVolcanoes(viewer: Viewer, volcanoes: Volcano[]) {
  const scaleByDistance = new NearFarScalar(1.0e5, 1.4, 2.0e7, 0.5);

  for (const v of volcanoes) {
    viewer.entities.add({
      id: `volcano-${v.id}`,
      name: v.name,
      position: Cartesian3.fromDegrees(v.longitude, v.latitude, v.elevation_m ?? 0),
      point: {
        pixelSize: 6,
        color: colorForStatus(v.status),
        outlineColor: Color.BLACK.withAlpha(0.55),
        outlineWidth: 1,
        scaleByDistance,
        // Sem isto os pontos do lado oculto do globo aparecem atraves da Terra.
        disableDepthTestDistance: 0,
      },
    });
  }
}

/**
 * Cor por evidencia de atividade, conforme o campo "Activity Evidence" do GVP.
 *
 * Isto e classificacao da propria fonte, nao um score calculado por nos: a §28
 * proibe apresentar resultado derivado como se fosse observacao.
 */
function colorForStatus(status?: string): Color {
  switch (status) {
    case "Eruption Observed":
      return Color.fromCssColorString("#ff5a3c");
    case "Eruption Dated":
      return Color.fromCssColorString("#ffa63c");
    case "Evidence Credible":
      return Color.fromCssColorString("#ffd93c");
    case "Evidence Uncertain":
      return Color.fromCssColorString("#9fb4c7");
    default:
      return Color.fromCssColorString("#7f93a8");
  }
}


/**
 * Liga o clique no globo a selecao de um vulcao, e mantem o balao colado ao
 * ponto enquanto a Terra gira.
 *
 * A posicao e recalculada em `preRender` porque a projecao muda a cada quadro:
 * fixar a coordenada no momento do clique faria o balao descolar do vulcao na
 * primeira rotacao.
 */
function wireSelection(
  viewer: Viewer,
  byId: Map<number, Volcano>,
  selectedIdRef: { current: number | null },
  setSelected: (
    s: { volcano: Volcano; x: number; y: number } | null,
  ) => void,
) {
  const handler = new ScreenSpaceEventHandler(viewer.scene.canvas);

  handler.setInputAction((movement: { position: Cartesian2 }) => {
    const picked = viewer.scene.pick(movement.position);
    const id = entityVolcanoId(picked);
    if (id == null) {
      // Clique no vazio fecha o balao: e o gesto que todo mapa ensina.
      selectedIdRef.current = null;
      setSelected(null);
      return;
    }
    selectedIdRef.current = id;
  }, ScreenSpaceEventType.LEFT_CLICK);

  viewer.scene.preRender.addEventListener(() => {
    const id = selectedIdRef.current;
    if (id == null) return;
    const volcano = byId.get(id);
    if (!volcano) return;

    const position = Cartesian3.fromDegrees(
      volcano.longitude,
      volcano.latitude,
      volcano.elevation_m ?? 0,
    );

    // Esconde o balao quando o vulcao esta do lado oculto do globo; sem isto
    // ele flutuaria sobre a Terra apontando para um ponto invisivel. O teste e
    // o sinal do produto escalar entre a normal da superficie no ponto e o
    // vetor que vai do ponto ate a camera.
    if (!facesCamera(position, viewer.scene.camera.position)) {
      setSelected(null);
      return;
    }

    const screen = SceneTransforms.worldToWindowCoordinates(
      viewer.scene,
      position,
    );
    if (!defined(screen)) return;
    setSelected({ volcano, x: screen.x, y: screen.y });
  });
}

/** Reporta se um ponto na superficie esta no hemisferio voltado para a camera. */
function facesCamera(point: Cartesian3, camera: Cartesian3): boolean {
  const normal = Ellipsoid.WGS84.geodeticSurfaceNormal(point, new Cartesian3());
  if (!defined(normal)) return true;
  const toCamera = Cartesian3.subtract(camera, point, new Cartesian3());
  return Cartesian3.dot(normal, toCamera) > 0;
}

function entityVolcanoId(picked: unknown): number | null {
  if (!defined(picked)) return null;
  const entityId = (picked as { id?: { id?: string } })?.id?.id;
  if (typeof entityId !== "string" || !entityId.startsWith("volcano-")) {
    return null;
  }
  const n = Number(entityId.slice("volcano-".length));
  return Number.isFinite(n) ? n : null;
}
