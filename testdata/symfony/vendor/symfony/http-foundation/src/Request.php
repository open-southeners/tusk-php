<?php

namespace Symfony\Component\HttpFoundation;

class Request
{
    /**
     * Gets a request parameter value.
     *
     * @param string $key
     * @param mixed $default
     * @return mixed
     */
    public function get(string $key, mixed $default = null): mixed
    {
        return $default;
    }

    /**
     * Gets the request content.
     *
     * @return string
     */
    public function getContent(): string
    {
        return '';
    }

    /**
     * Gets the request method.
     *
     * @return string
     */
    public function getMethod(): string
    {
        return 'GET';
    }

    /**
     * Gets the request URI.
     *
     * @return string
     */
    public function getUri(): string
    {
        return '';
    }
}
