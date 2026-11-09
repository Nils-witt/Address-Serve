import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { GeoObjectInput } from '../api/types';
import ErrorBanner from '../components/ErrorBanner';
import GeoObjectDialog from '../features/geo-objects/GeoObjectDialog';
import { useApi } from '../hooks/useApi';
import { useConfirm } from '../hooks/useConfirm';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { useGeoObjects } from '../hooks/useGeoObjects';
import { useInvalidate } from '../hooks/useInvalidate';
import { useStreets } from '../hooks/useStreets';
import { errorMessage } from '../lib/errors';
import GeoObjectsListTable, { type GeoObjectRow } from './geo-objects-page/GeoObjectsListTable';
import GeoObjectsToolbar from './geo-objects-page/GeoObjectsToolbar';

const ALL_STREETS = {};

/** Orders rows by group, then name. */
function compareRows(a: GeoObjectRow, b: GeoObjectRow): number {
  return (
    a.geoObject.group.localeCompare(b.geoObject.group) ||
    a.geoObject.name.localeCompare(b.geoObject.name)
  );
}

export default function GeoObjectsPage() {
  const { t } = useTranslation();
  const api = useApi();
  const confirm = useConfirm();
  const invalidate = useInvalidate();

  // The filters live in the URL, so a filtered list can be linked and the
  // back button from a street returns to it.
  const [searchParams, setSearchParams] = useSearchParams();
  const group = searchParams.get('group') ?? '';
  const [name, setName] = useState(searchParams.get('name') ?? '');
  const debouncedName = useDebouncedValue(name.trim());

  const setFilter = (key: string, value: string) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value) next.set(key, value);
        else next.delete(key);
        return next;
      },
      { replace: true },
    );
  };

  const filter = useMemo(() => ({ name: debouncedName, group }), [debouncedName, group]);
  const geoObjects = useGeoObjects(filter);
  // For the street name and city of each row. Geo objects can't be filtered
  // by city, so every street is needed.
  const streets = useStreets(ALL_STREETS);

  const rows = useMemo(() => {
    const streetsById = new Map(streets.data.map((s) => [s.id, s]));
    return geoObjects.data
      .map((geoObject) => ({ geoObject, street: streetsById.get(geoObject.streetId) }))
      .sort(compareRows);
  }, [geoObjects.data, streets.data]);

  // The edited row stays set while the dialog closes, so its form doesn't
  // flip to "new geo object" during the exit animation.
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<GeoObjectRow | null>(null);
  const [error, setError] = useState<string | null>(null);

  const openDialog = (row: GeoObjectRow | null) => {
    setEditing(row);
    setDialogOpen(true);
  };

  const onSubmit = async (input: GeoObjectInput) => {
    if (editing) {
      await api.updateGeoObject(editing.geoObject.id, input);
    } else {
      await api.createGeoObject(input);
    }
    await invalidate('geoObjects');
  };

  const onDelete = async ({ geoObject }: GeoObjectRow) => {
    const ok = await confirm({
      title: t('geoObjects.deleteTitle'),
      message: t('geoObjects.deleteMessage', { name: geoObject.name, group: geoObject.group }),
      confirmLabel: t('common.delete'),
      destructive: true,
    });
    if (!ok) return;
    setError(null);
    try {
      await api.deleteGeoObject(geoObject.id);
      await invalidate('geoObjects');
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <>
      <GeoObjectsToolbar
        name={name}
        group={group}
        onNameChange={(value) => {
          setName(value);
          setFilter('name', value.trim());
        }}
        onGroupChange={(value) => setFilter('group', value)}
        onCreate={() => openDialog(null)}
      />
      <ErrorBanner message={error ?? geoObjects.error ?? streets.error} />
      <GeoObjectsListTable
        rows={rows}
        loading={geoObjects.loading || streets.loading}
        onEdit={openDialog}
        onDelete={(row) => void onDelete(row)}
      />
      <GeoObjectDialog
        open={dialogOpen}
        geoObject={editing?.geoObject ?? null}
        street={editing?.street ?? null}
        onClose={() => setDialogOpen(false)}
        onSubmit={onSubmit}
      />
    </>
  );
}
