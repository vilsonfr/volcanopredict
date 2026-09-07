import { useEffect, useRef, useState } from "react";
import {
  Viewer,
  Cartesian3,
  Math as CesiumMath,
  ShadowMode,
  Color,
} from "cesium";
import "cesium/Build/Cesium/Widgets/widgets.css";
import { buildImagery } from "./imagery";

type Status = "loading" | "ready" | "error";

export function Globe({
  onTerrainResolved,
}: {
  onTerrainResolved?: (hasRealTerrain: boolean) => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewerRef = useRef<Viewer | null>(null);
  const [status, setStatus] = useState<Status>("loading");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    let viewer: Viewer | null = null;
    let cancelled = false;

    (async () => {
      try {
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
        });
        viewerRef.current = viewer;

        viewer.imageryLayers.removeAll();
        viewer.imageryLayers.addImageryProvider(imageryProvider);

        const { scene } = viewer;
        // Ajustes que separam "esfera com textura" de algo que parece a Terra:
        // sombreamento solar real, atmosfera, e névoa com a distância.
        scene.globe.enableLighting = true;
        scene.globe.showGroundAtmosphere = true;
        scene.globe.depthTestAgainstTerrain = true;
        if (scene.skyAtmosphere) scene.skyAtmosphere.show = true;
        scene.fog.enabled = true;
        scene.shadowMap.enabled = false;
        scene.globe.shadows = ShadowMode.DISABLED;
        scene.backgroundColor = Color.BLACK;
        scene.highDynamicRange = false;

        viewer.camera.setView({
          destination: Cartesian3.fromDegrees(-20, 5, 24_000_000),
          orientation: {
            heading: 0,
            pitch: CesiumMath.toRadians(-90),
            roll: 0,
          },
        });

        onTerrainResolved?.(hasRealTerrain);
        setStatus("ready");
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
  }, [onTerrainResolved]);

  return (
    <div className="globe-root">
      <div ref={containerRef} className="globe-canvas" />
      {status === "loading" && (
        <div className="globe-overlay">Carregando o globo…</div>
      )}
      {status === "error" && (
        <div className="globe-overlay globe-overlay--error">
          Não foi possível iniciar o globo: {error}
        </div>
      )}
    </div>
  );
}
