import { useCallback } from "react";
import { useLocation, useNavigate, type Location } from "react-router-dom";
import { readDetailReturnTo, type DetailLocationState } from "../routing/detailReturnTo";

/** Navigation options that preserve `returnTo` when opening a row from a list that was opened in context (e.g. from an agent system graph). */
export function detailListNavState(location: Location): { state?: DetailLocationState } {
  const returnTo = readDetailReturnTo(location);
  return returnTo ? { state: { returnTo } } : {};
}

export function useDetailReturnNav(fallbackPath: string) {
  const location = useLocation();
  const navigate = useNavigate();
  const returnTo = readDetailReturnTo(location);
  const goBack = useCallback(() => {
    navigate(returnTo ?? fallbackPath);
  }, [navigate, returnTo, fallbackPath]);
  return { goBack };
}
