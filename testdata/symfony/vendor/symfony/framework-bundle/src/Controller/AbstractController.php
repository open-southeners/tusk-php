<?php

namespace Symfony\Bundle\FrameworkBundle\Controller;

use Symfony\Component\HttpFoundation\Response;
use Symfony\Component\HttpFoundation\JsonResponse;

abstract class AbstractController
{
    /**
     * Returns a JsonResponse that uses the serializer component if enabled, or json_encode.
     *
     * @param mixed $data
     * @param int $status
     * @param array $headers
     * @return JsonResponse
     */
    protected function json(mixed $data, int $status = 200, array $headers = []): JsonResponse
    {
        return new JsonResponse();
    }

    /**
     * Returns a rendered view.
     *
     * @param string $view
     * @param array $parameters
     * @return Response
     */
    protected function render(string $view, array $parameters = []): Response
    {
        return new Response();
    }

    /**
     * Returns a RedirectResponse to the given URL.
     *
     * @param string $url
     * @param int $status
     * @return Response
     */
    protected function redirect(string $url, int $status = 302): Response
    {
        return new Response();
    }

    /**
     * Get a service from the container.
     *
     * @template T
     * @param class-string<T> $id
     * @return T
     */
    protected function get(string $id): mixed
    {
        return null;
    }
}
